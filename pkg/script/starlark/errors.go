package starlark

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script"
	"github.com/cloudposse/atmos/pkg/script/starlark/internal/convert"
)

// fileOptions is the Starlark dialect shared by entrypoints and loaded modules. Top-level
// control flow, while loops, sets, and recursion are enabled; globals stay single-assignment.
var fileOptions = &syntax.FileOptions{TopLevelControl: true, While: true, Set: true, Recursion: true}

const (
	reassignGlobalMessage = "cannot reassign global"
	reassignGlobalHint    = "Starlark globals are assigned once. Use a conditional expression (`x = a if cond else b`) " +
		"or compute the value in a function (`def main(): ... return v` then `output = main()`)."
)

// failure is a single-line script error classified by a sentinel. Its message is exactly msg,
// so builtins never repeat the generic Starlark prefix; scriptError adds ErrStarlark once at
// the outermost layer. The detail is surfaced as an error-builder explanation.
type failure struct {
	kind   error
	msg    string
	detail string
	causes []error
}

// Error returns the single-line message, without any generic Starlark prefix.
func (f *failure) Error() string {
	defer perf.Track(nil, "starlark.failure.Error")()

	return f.msg
}

// Unwrap exposes the causes so errors.Is and errors.As see context errors and wrapped failures.
func (f *failure) Unwrap() []error {
	defer perf.Track(nil, "starlark.failure.Unwrap")()

	return f.causes
}

// Is makes every classified failure satisfy both its own sentinel and ErrStarlark.
func (f *failure) Is(target error) bool {
	defer perf.Track(nil, "starlark.failure.Is")()

	return target == errUtils.ErrStarlark || errors.Is(f.kind, target)
}

// ErrorDetail feeds the explanation section of the error formatter.
func (f *failure) ErrorDetail() string {
	defer perf.Track(nil, "starlark.failure.ErrorDetail")()

	return f.detail
}

// usageFailure carries a host-presented usage error through the interpreter unchanged. The user
// mistyped the command line; the script did not fail, so scriptError returns the wrapped error
// as-is, without the Starlark prefix or a traceback.
type usageFailure struct{ err error }

// Error returns the usage message.
func (u *usageFailure) Error() string {
	defer perf.Track(nil, "starlark.usageFailure.Error")()

	return u.err.Error()
}

// Unwrap exposes the usage error so errors.Is sees ErrScriptUsage.
func (u *usageFailure) Unwrap() error {
	defer perf.Track(nil, "starlark.usageFailure.Unwrap")()

	return u.err
}

// fail creates a classified failure without a cause.
func fail(kind error, format string, args ...any) error {
	return &failure{kind: kind, msg: fmt.Sprintf(format, args...)}
}

// failWith creates a classified failure that wraps cause and inherits its explanations.
func failWith(kind, cause error, format string, args ...any) error {
	return &failure{
		kind: kind, msg: fmt.Sprintf(format, args...), causes: []error{cause},
		detail: joinDetails(cause),
	}
}

// failWithAll is failWith for several causes; nil causes are ignored.
func failWithAll(kind error, causes []error, format string, args ...any) error {
	f := &failure{kind: kind, msg: fmt.Sprintf(format, args...)}
	details := make([]string, 0, len(causes))
	for _, cause := range causes {
		if cause == nil {
			continue
		}
		f.causes = append(f.causes, cause)
		if detail := joinDetails(cause); detail != "" {
			details = append(details, detail)
		}
	}
	f.detail = strings.Join(details, "\n\n")
	return f
}

// withContext keeps cancellation and deadline errors reachable through errors.Is even when
// Starlark reports them as plain evaluation errors.
func withContext(ctx context.Context, err error) error {
	if err == nil || ctx == nil || ctx.Err() == nil || errors.Is(err, ctx.Err()) {
		return err
	}
	return failWithAll(errUtils.ErrStarlark, []error{err, ctx.Err()}, "%s", err)
}

// invalidArg reports an argument validation failure from a builtin.
func invalidArg(format string, args ...any) error {
	return convert.InvalidArgument(format, args...)
}

// withDetail attaches an explanation to a failure created by this package.
func withDetail(err error, detail string) error {
	var f *failure
	if errors.As(err, &f) {
		if f.detail != "" {
			detail = f.detail + "\n\n" + detail
		}
		f.detail = detail
	}
	return err
}

// joinDetails collects explanations along the single-cause chain, innermost first.
func joinDetails(err error) string {
	var details []string
	for ; err != nil; err = errors.Unwrap(err) {
		if detailer, ok := err.(interface{ ErrorDetail() string }); ok && detailer.ErrorDetail() != "" {
			details = append(details, detailer.ErrorDetail())
		}
	}
	slices.Reverse(details)
	return strings.Join(details, "\n\n")
}

func fenced(text string) string {
	return "```text\n" + strings.TrimRight(text, "\n") + "\n```"
}

// scriptError is the single place where ErrStarlark joins the chain. EvalErrors contribute
// their one-line message plus the backtrace as a fenced explanation; cancellation and
// deadline errors stay reachable through errors.Is. Paths under projectRoot are shown relative
// to it (display only).
func scriptError(ctx context.Context, err error, projectRoot string) error {
	var usage *usageFailure
	if errors.As(err, &usage) {
		return usage.err
	}
	cause := withContext(ctx, err)
	recursion := isRecursionError(err)
	if recursion && !errors.Is(err, errUtils.ErrStarlarkRecursionLimit) {
		cause = recursionFailure(cause)
	}
	builder := errUtils.Build(errUtils.ErrStarlark).WithCause(displayError(projectRoot, cause))
	script.EnrichDiagnostics(builder, cause)
	var eval *starlark.EvalError
	if errors.As(err, &eval) {
		builder.WithExplanation(fenced(backtraceText(eval, projectRoot)))
	}
	if recursion {
		builder.WithHint(recursionHint)
	}
	if strings.Contains(err.Error(), reassignGlobalMessage) {
		builder.WithHint(reassignGlobalHint)
	}
	return builder.Err()
}

// backtraceText renders the backtrace of an evaluation error for display: project-relative paths,
// and long runs of repeated frames collapsed into a single marker line.
func backtraceText(eval *starlark.EvalError, projectRoot string) string {
	return collapseBacktrace(displayPaths(projectRoot, eval.Backtrace()))
}

// pathBoundary matches the character before a path in an error message or backtrace, so a root is
// only recognized at the start of a path and never in the middle of another one.
const pathBoundary = `(^|[\s"'(\[=,;])`

// displayPaths rewrites absolute paths under projectRoot to paths relative to it. Only a prefix that
// is exactly the root followed by a separator is rewritten, so the result never contains `..` and
// paths outside the root stay absolute. It returns text unchanged when projectRoot is empty.
func displayPaths(projectRoot, text string) string {
	if projectRoot == "" || text == "" {
		return text
	}
	prefix := strings.TrimRight(filepath.Clean(projectRoot), string(filepath.Separator)) + string(filepath.Separator)
	if !strings.Contains(text, prefix) {
		return text
	}
	return regexp.MustCompile(pathBoundary+regexp.QuoteMeta(prefix)).ReplaceAllString(text, "${1}")
}

// relativeError carries a message with project-relative paths while keeping the original error
// reachable through errors.Is and errors.As.
type relativeError struct {
	err error
	msg string
}

func (e *relativeError) Error() string {
	defer perf.Track(nil, "starlark.relativeError.Error")()

	return e.msg
}

func (e *relativeError) Unwrap() error {
	defer perf.Track(nil, "starlark.relativeError.Unwrap")()

	return e.err
}

// displayError returns err with project-relative paths in its message, or err itself when there
// is nothing to rewrite.
func displayError(projectRoot string, err error) error {
	if err == nil || projectRoot == "" {
		return err
	}
	msg := err.Error()
	if shown := displayPaths(projectRoot, msg); shown != msg {
		return &relativeError{err: err, msg: shown}
	}
	return err
}

// evalMessage returns the single-line message of an evaluation error without its backtrace.
func evalMessage(err error) string {
	var eval *starlark.EvalError
	if errors.As(err, &eval) {
		return eval.Msg
	}
	return err.Error()
}

// evalDetail returns the backtrace explanation of err, if it carries one. Paths under projectRoot
// are shown relative to it.
func evalDetail(err error, projectRoot string) string {
	var eval *starlark.EvalError
	if errors.As(err, &eval) {
		return fenced(backtraceText(eval, projectRoot))
	}
	return ""
}
