package starlark

import (
	"errors"
	"slices"
	"strings"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script"
	"github.com/cloudposse/atmos/pkg/script/starlark/internal/convert"
	"github.com/cloudposse/atmos/pkg/ui"
)

// ciModuleValue is the predeclared `ci` module. It is a custom value rather than a plain
// module so `ci.context` resolves lazily on first read: a script that never touches it never
// asks the reporter for the run context. Every other member is a builtin.
//
// The value holds no mutable script state, so Freeze is a no-op and steps.parallel freezing the
// predeclared names cannot break later `ci.*` calls from tasks.
type ciModuleValue struct {
	s       *session
	members starlark.StringDict
}

var _ starlark.HasAttrs = (*ciModuleValue)(nil)

// ciContextAttr is the lazily resolved member of the module.
const ciContextAttr = "context"

// ciArgName is the name of the identifying argument shared by output, env, and check.
const ciArgName = "name"

// ciModule builds the `ci` module.
func (s *session) ciModule() starlark.Value {
	return &ciModuleValue{s: s, members: starlark.StringDict{
		"summary":  starlark.NewBuiltin("ci.summary", s.ciSummary),
		"output":   starlark.NewBuiltin("ci.output", s.ciOutput),
		"env":      starlark.NewBuiltin("ci.env", s.ciEnv),
		"path":     starlark.NewBuiltin("ci.path", s.ciPath),
		"mask":     starlark.NewBuiltin("ci.mask", s.ciMask),
		"annotate": starlark.NewBuiltin("ci.annotate", s.ciAnnotate),
		"comment":  starlark.NewBuiltin("ci.comment", s.ciComment),
		"check":    starlark.NewBuiltin("ci.check", s.ciCheck),
		"group":    starlark.NewBuiltin("ci.group", s.ciGroup),
		"sarif":    starlark.NewBuiltin("ci.sarif", s.ciSARIF),
		"base":     starlark.NewBuiltin("ci.base", s.ciBase),
	}}
}

// String names the module.
func (m *ciModuleValue) String() string {
	defer perf.Track(nil, "starlark.ciModuleValue.String")()

	return "<module ci>"
}

// Type matches the type of the other modules.
func (m *ciModuleValue) Type() string {
	defer perf.Track(nil, "starlark.ciModuleValue.Type")()

	return "module"
}

// Freeze is a no-op: all mutable state lives in Go behind locks.
func (m *ciModuleValue) Freeze() {
	defer perf.Track(nil, "starlark.ciModuleValue.Freeze")()
}

// Truth reports that the module is always truthy.
func (m *ciModuleValue) Truth() starlark.Bool {
	defer perf.Track(nil, "starlark.ciModuleValue.Truth")()

	return true
}

// Hash is unsupported, like other modules.
func (m *ciModuleValue) Hash() (uint32, error) {
	defer perf.Track(nil, "starlark.ciModuleValue.Hash")()

	return 0, invalidArg("unhashable type: module")
}

// Attr returns a builtin, or resolves the run context on first read of `context`.
func (m *ciModuleValue) Attr(name string) (starlark.Value, error) {
	defer perf.Track(nil, "starlark.ciModuleValue.Attr")()

	if name == ciContextAttr {
		return m.s.ciContextValue(), nil
	}
	if value, ok := m.members[name]; ok {
		return value, nil
	}
	return nil, nil
}

// AttrNames lists every member in sorted order.
func (m *ciModuleValue) AttrNames() []string {
	defer perf.Track(nil, "starlark.ciModuleValue.AttrNames")()

	names := append(m.members.Keys(), ciContextAttr)
	slices.Sort(names)
	return names
}

// reporter binds the session reporter's local renderings to the calling thread's stderr, so
// output from parallel tasks is line-atomic and prefixed like other task output.
func (s *session) reporter(t *starlark.Thread) ci.Reporter {
	return s.ciReporter.WithOutput(s.writer(t, stderrStream))
}

// ciReport warns when a detected provider was skipped because a switch is off. Local renderings
// without a gate are the expected outcome outside CI and stay silent.
func (s *session) ciReport(t *starlark.Thread, name string, rc ci.Receipt) {
	if rc.Gate == "" {
		return
	}
	ui.New(s.writer(t, stderrStream)).Warningf("ci.%s: %s is off; rendered locally", name, rc.Gate)
}

// ciSentinels are the reporter failures worth classifying individually, most specific first.
var ciSentinels = []error{
	errUtils.ErrCIPullRequestUnknown,
	errUtils.ErrCICommentPostFailed,
	errUtils.ErrCICheckRunCreateFailed,
	errUtils.ErrCICheckRunUpdateFailed,
	errUtils.ErrCIOutputWriteFailed,
	errUtils.ErrCISummaryWriteFailed,
	errUtils.ErrCIAnnotationFailed,
	errUtils.ErrCISARIFUploadFailed,
	errUtils.ErrCIEnvWriteFailed,
	errUtils.ErrCIMaskFailed,
	errUtils.ErrCIProviderNotDetected,
}

// ciFail converts a reporter error into a script failure classified by the matching CI sentinel.
func ciFail(name string, err error) error {
	kind := errUtils.ErrStarlark
	for _, sentinel := range ciSentinels {
		if errors.Is(err, sentinel) {
			kind = sentinel
			break
		}
	}
	if !errors.Is(err, errUtils.ErrCIPullRequestUnknown) {
		return failWith(kind, err, "ci.%s: %v", name, err)
	}
	message := "ci." + name + ": " + err.Error()
	hint := script.NewDiagnostic(message).With(func(b *errUtils.ErrorBuilder) {
		b.WithHint("Pass the pull request number, for example pr=123, or run in a pull request context")
	}).Err()
	return failWithAll(kind, []error{hint, err}, "%s", message)
}

// ciDone finishes a reporter write: it maps the error or reports a gated receipt, and returns None.
func (s *session) ciDone(t *starlark.Thread, name string, rc ci.Receipt, err error) (starlark.Value, error) {
	if err != nil {
		return nil, ciFail(name, err)
	}
	s.ciReport(t, name, rc)
	return starlark.None, nil
}

// unpackCI unpacks arguments and classifies any mismatch as an invalid argument.
func unpackCI(b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple, pairs ...any) error {
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, pairs...); err != nil {
		return convert.ArgumentCause(err, "%s", err.Error())
	}
	return nil
}

// requireText rejects an empty required string argument.
func requireText(b *starlark.Builtin, field, value string) error {
	if value == "" {
		return invalidArg("%s: %s must not be empty", b.Name(), field)
	}
	return nil
}

// oneOf validates value against the allowed choices and lists them on failure.
func oneOf[T ~string](b *starlark.Builtin, field, value string, allowed ...T) (T, error) {
	for _, choice := range allowed {
		if string(choice) == value {
			return choice, nil
		}
	}
	names := make([]string, len(allowed))
	for i, choice := range allowed {
		names[i] = string(choice)
	}
	var zero T
	return zero, invalidArg("%s: %s must be one of %s, got %q", b.Name(), field, strings.Join(names, ", "), value)
}

func (s *session) ciSummary(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var markdown string
	if err := unpackCI(b, args, kwargs, "markdown", &markdown); err != nil {
		return nil, err
	}
	rc, err := s.reporter(t).Summary(markdown)
	return s.ciDone(t, "summary", rc, err)
}

func (s *session) ciOutput(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, value string
	if err := unpackCI(b, args, kwargs, "name", &name, "value", &value); err != nil {
		return nil, err
	}
	if err := requireText(b, ciArgName, name); err != nil {
		return nil, err
	}
	rc, err := s.reporter(t).Output(name, value)
	return s.ciDone(t, "output", rc, err)
}

func (s *session) ciEnv(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, value string
	if err := unpackCI(b, args, kwargs, "name", &name, "value", &value); err != nil {
		return nil, err
	}
	if err := requireText(b, ciArgName, name); err != nil {
		return nil, err
	}
	rc, err := s.reporter(t).Env(name, value)
	return s.ciDone(t, "env", rc, err)
}

func (s *session) ciPath(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var dir string
	if err := unpackCI(b, args, kwargs, "dir", &dir); err != nil {
		return nil, err
	}
	if err := requireText(b, "dir", dir); err != nil {
		return nil, err
	}
	rc, err := s.reporter(t).Path(dir)
	return s.ciDone(t, "path", rc, err)
}

func (s *session) ciMask(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var value string
	if err := unpackCI(b, args, kwargs, "value", &value); err != nil {
		return nil, err
	}
	rc, err := s.reporter(t).Mask(value)
	return s.ciDone(t, "mask", rc, err)
}

func (s *session) ciAnnotate(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var level, message, file, title string
	var line, endLine int
	if err := unpackCI(b, args, kwargs, "level", &level, "message", &message, "file?", &file, "line?", &line, "end_line?", &endLine, "title?", &title); err != nil {
		return nil, err
	}
	parsed, err := oneOf(b, "level", level, ci.AnnotationError, ci.AnnotationWarning, ci.AnnotationNotice)
	if err != nil {
		return nil, err
	}
	if err := requireText(b, "message", message); err != nil {
		return nil, err
	}
	if line < 0 || endLine < 0 {
		return nil, invalidArg("%s: line and end_line must not be negative", b.Name())
	}
	rc, err := s.reporter(t).Annotate(ci.Annotation{
		Path: file, StartLine: line, EndLine: endLine, Level: parsed, Title: title, Message: message,
	})
	return s.ciDone(t, "annotate", rc, err)
}

func (s *session) ciComment(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var body, key string
	behavior := string(ci.CommentBehaviorUpsert)
	var pr int
	if err := unpackCI(b, args, kwargs, "body", &body, "key?", &key, "behavior?", &behavior, "pr?", &pr); err != nil {
		return nil, err
	}
	if err := requireText(b, "body", body); err != nil {
		return nil, err
	}
	parsed, err := oneOf(b, "behavior", behavior, ci.CommentBehaviorCreate, ci.CommentBehaviorUpdate, ci.CommentBehaviorUpsert)
	if err != nil {
		return nil, err
	}
	if pr < 0 {
		return nil, invalidArg("%s: pr must not be negative", b.Name())
	}
	rc, err := s.reporter(t).Comment(threadContext(t), ci.CommentRequest{Body: body, Key: key, Behavior: parsed, PR: pr})
	if err != nil {
		return nil, ciFail("comment", err)
	}
	s.ciReport(t, "comment", rc)
	return commentValue(rc.Comment), nil
}

func (s *session) ciGroup(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var title string
	var fn starlark.Value
	if err := unpackCI(b, args, kwargs, "title", &title, "fn", &fn); err != nil {
		return nil, err
	}
	callable, ok := fn.(starlark.Callable)
	if !ok {
		return nil, invalidArg("%s: fn must be callable, got %s", b.Name(), fn.Type())
	}
	end, rc, err := s.reporter(t).Group(title)
	if err != nil {
		return nil, ciFail("group", err)
	}
	defer end()
	s.ciReport(t, "group", rc)
	return starlark.Call(t, callable, nil, nil)
}

func (s *session) ciSARIF(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path, category string
	if err := unpackCI(b, args, kwargs, "path", &path, "category?", &category); err != nil {
		return nil, err
	}
	if err := requireText(b, "path", path); err != nil {
		return nil, err
	}
	body, err := s.engine.readFile(s.filesystemPath(path))
	if err != nil {
		return nil, failWith(errUtils.ErrStarlark, err, "ci.sarif: cannot read file: %s", err)
	}
	rc, err := s.reporter(t).SARIF(threadContext(t), ci.SARIFReport{Body: body, Category: category, Path: path})
	return s.ciDone(t, "sarif", rc, err)
}
