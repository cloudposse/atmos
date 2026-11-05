// Package cast provides declarative terminal recordings to embedded Starlark.
package cast

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"

	starjson "go.starlark.net/lib/json"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/cloudposse/atmos/pkg/castplan"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script/starlark/internal/convert"
)

// Host supplies execution and read-only validation without coupling the binding to a step runner.
type Host interface {
	Check(*starlark.Thread, *castplan.Plan) error
	Generate(*starlark.Thread, *castplan.Plan, starlark.Callable) (string, error)
	Validate(*starlark.Thread, *castplan.Plan, starlark.Callable) (string, error)
}

const (
	maxExitCode    = 255
	textField      = "text"
	simulateAction = "simulate"
)

// New creates a module backed by invocation-local host services.
func New(host Host) starlark.Value {
	defer perf.Track(nil, "cast.New")()

	return &starlarkstruct.Module{Name: "cast", Members: starlark.StringDict{
		"recorder": starlark.NewBuiltin("cast.recorder", func(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			return newRecorder(t, host, args, kwargs)
		}),
		"regex": starlark.NewBuiltin("cast.regex", newRegex),
	}}
}

func newRecorder(t *starlark.Thread, host Host, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 0 {
		return nil, convert.InvalidArgument("cast.recorder accepts keyword arguments")
	}
	fields, policy, err := configuration(t, kwargs)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"steps", "tape", "source", "mode", "type"} {
		if _, exists := fields[key]; exists {
			return nil, convert.InvalidArgument("cast.recorder declares its own actions; %s is not a constructor option", key)
		}
	}
	if _, ok := fields["output"].(string); ok {
		fields["output"] = map[string]any{"cast": fields["output"], "mode": "raw"}
	}
	replacements, err := recordingReplacements(fields)
	if err != nil {
		return nil, err
	}
	return &recorder{host: host, plan: &castplan.Plan{Configuration: fields, Replacements: replacements}, policy: policy, dir: stringField(fields, "working_directory")}, nil
}

func recordingReplacements(fields map[string]any) (map[string]string, error) {
	replacements := map[string]string{}
	raw, exists := fields["replacements"]
	if !exists {
		return replacements, nil
	}
	values, ok := raw.(map[string]any)
	if !ok {
		return nil, convert.InvalidArgument("replacements must be a string dictionary")
	}
	for key, value := range values {
		text, ok := value.(string)
		if !ok || key == "" {
			return nil, convert.InvalidArgument("replacements requires nonempty keys and string values")
		}
		replacements[key] = text
	}
	delete(fields, "replacements")
	return replacements, nil
}

func configuration(t *starlark.Thread, kwargs []starlark.Tuple) (map[string]any, starlark.Callable, error) {
	var policy starlark.Callable
	fields := starlark.NewDict(len(kwargs))
	for _, pair := range kwargs {
		key := string(pair[0].(starlark.String))
		if key == "policy" {
			if pair[1] != starlark.None {
				var ok bool
				policy, ok = pair[1].(starlark.Callable)
				if !ok {
					return nil, nil, convert.InvalidArgument("policy must be a function accepting recorded text")
				}
			}
			continue
		}
		if key == "with_" {
			key = "with"
		}
		if err := fields.SetKey(starlark.String(key), pair[1]); err != nil {
			return nil, nil, err
		}
	}
	value, err := starlark.Call(t, starjson.Module.Members["encode"], starlark.Tuple{fields}, nil)
	if err != nil {
		return nil, nil, err
	}
	var result map[string]any
	err = json.Unmarshal([]byte(string(value.(starlark.String))), &result)
	return result, policy, err
}

type regexMatcher struct{ pattern string }

func (m *regexMatcher) String() string {
	defer perf.Track(nil, "cast.regexMatcher.String")()

	return "cast.regex(" + starlark.String(m.pattern).String() + ")"
}

func (m *regexMatcher) Type() string {
	defer perf.Track(nil, "cast.regexMatcher.Type")()

	return "cast.regex"
}

func (m *regexMatcher) Freeze() {
	defer perf.Track(nil, "cast.regexMatcher.Freeze")()
}

func (m *regexMatcher) Truth() starlark.Bool {
	defer perf.Track(nil, "cast.regexMatcher.Truth")()

	return starlark.True
}

func (m *regexMatcher) Hash() (uint32, error) {
	defer perf.Track(nil, "cast.regexMatcher.Hash")()

	return starlark.String(m.pattern).Hash()
}

func newRegex(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var pattern string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "pattern", &pattern); err != nil {
		return nil, err
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return nil, convert.ArgumentCause(err, "invalid cast regex: %s", err)
	}
	return &regexMatcher{pattern: pattern}, nil
}

func matchers(value starlark.Value) ([]castplan.Matcher, error) {
	switch value := value.(type) {
	case starlark.String:
		return []castplan.Matcher{{Text: string(value)}}, nil
	case *regexMatcher:
		return []castplan.Matcher{{Text: value.pattern, Regex: true}}, nil
	default:
		values, err := convert.Sequence(value)
		if err != nil {
			return nil, convert.InvalidArgument("expect/reject requires a string, cast.regex, or list of matchers")
		}
		result := []castplan.Matcher{}
		for _, item := range values {
			switch item.(type) {
			case starlark.String, *regexMatcher:
			default:
				return nil, convert.InvalidArgument("expect/reject lists must contain strings or cast.regex matchers")
			}
			matches, err := matchers(item)
			if err != nil {
				return nil, err
			}
			result = append(result, matches...)
		}
		return result, nil
	}
}

func stringField(fields map[string]any, key string) string {
	value, _ := fields[key].(string)
	return value
}

func actionFields(t *starlark.Thread, kind string, args starlark.Tuple, kwargs []starlark.Tuple) (castplan.Action, error) {
	action := castplan.Action{Kind: kind}
	fields, err := actionKeywords(&action, kwargs)
	if err != nil {
		return action, err
	}
	fields, err = actionPositional(kind, args, fields)
	if err != nil {
		return action, err
	}
	var policy starlark.Callable
	action.Fields, policy, err = configuration(t, fields)
	if policy != nil {
		return action, convert.InvalidArgument("policy belongs on cast.recorder")
	}
	if err != nil {
		return action, err
	}
	if kind != "run" && (len(action.Expect) > 0 || len(action.Reject) > 0 || action.ExitCode != 0) {
		return action, convert.InvalidArgument("expect, reject, and exit_code belong on rec.run")
	}
	return action, nil
}

func actionKeywords(action *castplan.Action, kwargs []starlark.Tuple) ([]starlark.Tuple, error) {
	fields := []starlark.Tuple{}
	for _, pair := range kwargs {
		key := string(pair[0].(starlark.String))
		switch key {
		case "expect", "reject":
			value, err := matchers(pair[1])
			if err != nil {
				return nil, err
			}
			if key == "expect" {
				action.Expect = value
			} else {
				action.Reject = value
			}
		case "exit_code":
			code, err := starlark.AsInt32(pair[1])
			if err != nil {
				return nil, err
			}
			if code < 0 || code > maxExitCode {
				return nil, convert.InvalidArgument("exit_code must be between 0 and 255")
			}
			action.ExitCode = code
		default:
			fields = append(fields, pair)
		}
	}
	return fields, nil
}

func actionPositional(kind string, args starlark.Tuple, fields []starlark.Tuple) ([]starlark.Tuple, error) {
	if len(args) == 0 {
		return fields, nil
	}
	if len(args) > 1 {
		return nil, convert.InvalidArgument("%s accepts at most one positional argument", kind)
	}
	positional := map[string]string{"run": "command", simulateAction: textField, "comment": textField, "cd": "path", "pause": "duration", "screenshot": "path", "step": "type", "write": textField, "send": textField, "key": "key", "wait": textField}
	name, ok := positional[kind]
	if !ok {
		return nil, convert.InvalidArgument("%s accepts keyword arguments only", kind)
	}
	for _, pair := range fields {
		if string(pair[0].(starlark.String)) == name {
			return nil, convert.InvalidArgument("duplicate %s", name)
		}
	}
	if matcher, ok := args[0].(*regexMatcher); ok && kind == "wait" {
		return append(fields, starlark.Tuple{starlark.String("regex"), starlark.String(matcher.pattern)}), nil
	}
	return append(fields, starlark.Tuple{starlark.String(name), args[0]}), nil
}

// relativeDirectory resolves a declared transition without touching the filesystem.
func relativeDirectory(base, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(base, path)
}

func commentText(text string) string {
	return "# " + strings.ReplaceAll(text, "\n", "\n# ")
}
