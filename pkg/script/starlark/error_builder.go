package starlark

import (
	"regexp"
	"slices"
	"strings"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script"
)

const maxScriptExitCode = 255

var errorContextKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// errorBuilderValue is immutable: every enrichment returns a new value, so a
// shared error template can safely be used by loaded modules and parallel tasks.
type errorBuilderValue struct {
	diagnostic *script.Diagnostic
}

func (v *errorBuilderValue) String() string {
	defer perf.Track(nil, "starlark.errorBuilderValue.String")()
	return "<error: " + v.diagnostic.Message() + ">"
}

func (v *errorBuilderValue) Type() string {
	defer perf.Track(nil, "starlark.errorBuilderValue.Type")()
	return "error_builder"
}

func (v *errorBuilderValue) Truth() starlark.Bool {
	defer perf.Track(nil, "starlark.errorBuilderValue.Truth")()
	return starlark.True
}

func (v *errorBuilderValue) Hash() (uint32, error) {
	defer perf.Track(nil, "starlark.errorBuilderValue.Hash")()
	return 0, invalidArg("error_builder is unhashable")
}

func (v *errorBuilderValue) Freeze() {
	defer perf.Track(nil, "starlark.errorBuilderValue.Freeze")()
}

func (v *errorBuilderValue) AttrNames() []string {
	defer perf.Track(nil, "starlark.errorBuilderValue.AttrNames")()
	return []string{"fail", "with_cause", "with_context", "with_example", "with_exit_code", "with_explanation", "with_hint", "with_title"}
}

func (v *errorBuilderValue) Attr(name string) (starlark.Value, error) {
	defer perf.Track(nil, "starlark.errorBuilderValue.Attr")()
	if !slices.Contains(v.AttrNames(), name) {
		return nil, nil
	}
	return starlark.NewBuiltin("error_builder."+name, func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		switch name {
		case "fail":
			if err := starlark.UnpackArgs(b.Name(), args, kwargs); err != nil {
				return nil, err
			}
			return nil, v.diagnostic.Err()
		case "with_context":
			return v.withContext(b, args, kwargs)
		case "with_cause":
			return v.withCause(b, args, kwargs)
		case "with_exit_code":
			return v.withExitCode(b, args, kwargs)
		default:
			return v.withText(name, b, args, kwargs)
		}
	}), nil
}

func (v *errorBuilderValue) with(enrich func(*errUtils.ErrorBuilder)) *errorBuilderValue {
	return &errorBuilderValue{diagnostic: v.diagnostic.With(enrich)}
}

func (v *errorBuilderValue) withCause(b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var cause starlark.Value
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "cause", &cause); err != nil {
		return nil, err
	}
	var value *errorBuilderValue
	switch cause := cause.(type) {
	case *errorBuilderValue:
		value = cause
	case starlark.String:
		if strings.TrimSpace(string(cause)) == "" {
			return nil, invalidArg("with_cause: cause must not be empty")
		}
		value = &errorBuilderValue{diagnostic: script.NewDiagnostic(string(cause))}
	default:
		return nil, invalidArg("with_cause: cause must be a string or error builder, got %s", cause.Type())
	}
	return &errorBuilderValue{diagnostic: v.diagnostic.WithCause(value.diagnostic.Err())}, nil
}

func (v *errorBuilderValue) withText(name string, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var text string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, strings.TrimPrefix(name, "with_"), &text); err != nil {
		return nil, err
	}
	methods := map[string]func(*errUtils.ErrorBuilder, string) *errUtils.ErrorBuilder{
		"with_title":       (*errUtils.ErrorBuilder).WithTitle,
		"with_hint":        (*errUtils.ErrorBuilder).WithHint,
		"with_explanation": (*errUtils.ErrorBuilder).WithExplanation,
		"with_example":     (*errUtils.ErrorBuilder).WithExample,
	}
	return v.with(func(builder *errUtils.ErrorBuilder) { methods[name](builder, text) }), nil
}

func (v *errorBuilderValue) withContext(b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key string
	var value starlark.Value
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "key", &key, "value", &value); err != nil {
		return nil, err
	}
	if !errorContextKey.MatchString(key) {
		return nil, invalidArg("with_context: key must be an identifier")
	}
	var text string
	switch value.(type) {
	case starlark.String:
		text, _ = starlark.AsString(value)
	case starlark.Int, starlark.Float, starlark.Bool, starlark.NoneType:
		text = value.String()
	default:
		return nil, invalidArg("with_context: value must be a string, number, bool, or None, got %s", value.Type())
	}
	return v.with(func(builder *errUtils.ErrorBuilder) { builder.WithContext(key, text) }), nil
}

func (v *errorBuilderValue) withExitCode(b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var code int
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "code", &code); err != nil {
		return nil, err
	}
	if code < 1 || code > maxScriptExitCode {
		return nil, invalidArg("with_exit_code: code must be between 1 and %d", maxScriptExitCode)
	}
	return v.with(func(builder *errUtils.ErrorBuilder) { builder.WithExitCode(code) }), nil
}

func buildError(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var message string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "message", &message); err != nil {
		return nil, err
	}
	if strings.TrimSpace(message) == "" {
		return nil, invalidArg("errors.build: message must not be empty")
	}
	return &errorBuilderValue{diagnostic: script.NewDiagnostic(message)}, nil
}
