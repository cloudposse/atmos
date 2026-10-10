package starlark

import (
	"strings"

	"go.starlark.net/starlark"
)

// ciTextSource describes the text arguments of ci.summary and ci.comment: the literal text, or a
// template with data that renders it.
type ciTextSource struct {
	// field is the name of the literal text argument, markdown or body.
	field string
	// value is the literal text argument, or nil when absent.
	value starlark.Value
	// template is the template name argument, or nil when absent.
	template starlark.Value
	// data is the template context argument, or nil when absent.
	data starlark.Value
	// render renders the named template with the data.
	render func(name string, data any) (string, error)
}

// resolveCIText returns the text to write. Literal text passes through unchanged. A template with
// optional data is rendered instead. There is no configured default template: data without
// template= is an argument error. Literal text and a template (or data) are mutually exclusive,
// decided by whether the argument was passed, not by its value, so an empty literal does not
// slip past the check.
func resolveCIText(b *starlark.Builtin, src *ciTextSource) (string, error) {
	text, hasText, err := ciLiteralText(b, src.field, src.value)
	if err != nil {
		return "", err
	}
	name, hasTemplate, err := ciTemplateName(b, src.template)
	if err != nil {
		return "", err
	}
	data, hasData, err := ciTemplateData(b, src.data)
	if err != nil {
		return "", err
	}
	if err := checkCITextArguments(b, src, hasText, hasTemplate, hasData); err != nil {
		return "", err
	}
	if !hasTemplate {
		return text, nil
	}
	rendered, err := src.render(name, data)
	if err != nil {
		return "", ciFail(strings.TrimPrefix(b.Name(), "ci."), err)
	}
	return rendered, nil
}

// checkCITextArguments rejects argument combinations that have no meaning: literal text with a
// template or data, data without a template, and nothing to write at all.
func checkCITextArguments(b *starlark.Builtin, src *ciTextSource, hasText, hasTemplate, hasData bool) error {
	if hasText && hasTemplate {
		return invalidArg("%s: %s and template are mutually exclusive", b.Name(), src.field)
	}
	if hasText && hasData {
		return invalidArg("%s: %s and data are mutually exclusive", b.Name(), src.field)
	}
	if hasData && !hasTemplate {
		return invalidArg("%s: data requires template=; no template is configured by default", b.Name())
	}
	if !hasText && !hasTemplate {
		return invalidArg("%s: %s is required unless template= is given", b.Name(), src.field)
	}
	return nil
}

// ciTemplateName reads the template argument, which must be a non-empty string when present. None means absent.
func ciTemplateName(b *starlark.Builtin, value starlark.Value) (name string, present bool, err error) {
	if value == nil || value == starlark.None {
		return "", false, nil
	}
	str, ok := value.(starlark.String)
	if !ok {
		return "", false, invalidArg("%s: for parameter template: got %s, want string", b.Name(), value.Type())
	}
	if str == "" {
		return "", false, invalidArg("%s: template must not be empty", b.Name())
	}
	return string(str), true, nil
}

// ciLiteralText reads the literal text argument, which must be a string when present.
func ciLiteralText(b *starlark.Builtin, field string, value starlark.Value) (text string, present bool, err error) {
	if value == nil {
		return "", false, nil
	}
	str, ok := value.(starlark.String)
	if !ok {
		return "", false, invalidArg("%s: for parameter %s: got %s, want string", b.Name(), field, value.Type())
	}
	return string(str), true, nil
}

// ciTemplateData converts the data argument to the template context. Absent and None mean no data.
func ciTemplateData(b *starlark.Builtin, value starlark.Value) (data any, present bool, err error) {
	if value == nil || value == starlark.None {
		return nil, false, nil
	}
	dict, ok := value.(*starlark.Dict)
	if !ok {
		return nil, false, invalidArg("%s: for parameter data: got %s, want dict", b.Name(), value.Type())
	}
	converted, err := configurationResult(dict, make(map[starlark.Value]bool))
	if err != nil {
		return nil, false, invalidArg("%s: data: %v", b.Name(), err)
	}
	return converted, true, nil
}
