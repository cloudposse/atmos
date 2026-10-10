package regex

import (
	"regexp"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script/starlark/internal/convert"
)

func compileRegex(pattern string) (*regexp.Regexp, error) {
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return nil, convert.ArgumentCause(err, "invalid regex: %s", err)
	}
	return expression, nil
}

func regexSearch(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var pattern, text string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "pattern", &pattern, "text", &text); err != nil {
		return nil, err
	}
	expression, err := compileRegex(pattern)
	if err != nil {
		return nil, err
	}
	return starlark.Bool(expression.MatchString(text)), nil
}

func regexReplace(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var pattern, replacement, text string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "pattern", &pattern, "replacement", &replacement, "text", &text); err != nil {
		return nil, err
	}
	expression, err := compileRegex(pattern)
	if err != nil {
		return nil, err
	}
	// Replacement is literal so paths and dollar signs need no extra escaping.
	return starlark.String(expression.ReplaceAllLiteralString(text, replacement)), nil
}

func regexFindAll(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var pattern, text string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "pattern", &pattern, "text", &text); err != nil {
		return nil, err
	}
	expression, err := compileRegex(pattern)
	if err != nil {
		return nil, err
	}
	matches := expression.FindAllString(text, -1)
	values := make([]starlark.Value, len(matches))
	for i, match := range matches {
		values[i] = starlark.String(match)
	}
	return starlark.NewList(values), nil
}

// New returns the stateless regular-expression module.
func New() starlark.Value {
	defer perf.Track(nil, "regex.New")()

	return &starlarkstruct.Module{Name: "regex", Members: starlark.StringDict{
		"search":  starlark.NewBuiltin("regex.search", regexSearch),
		"replace": starlark.NewBuiltin("regex.replace", regexReplace),
		"findall": starlark.NewBuiltin("regex.findall", regexFindAll),
	}}
}
