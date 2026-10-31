// Package embedded evaluates YAML functions found inside larger strings. Templates
// render before YAML functions, so a template can place a function's source inside
// other text. The caller supplies evaluation, which keeps this package free of
// configuration and stack processing.
package embedded

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Evaluator processes one string as a YAML function value.
type Evaluator func(input string) (any, error)

// Resolve substitutes each function in input with its value, so
// "app-{{ .vars.region }}" produces the function's value inside the surrounding text.
func Resolve(input string, evaluate Evaluator) (string, error) {
	defer perf.Track(nil, "embedded.Resolve")()

	if yamlsource.Transformed(input) {
		return "", fmt.Errorf("%w: a template function changed the value before the YAML function could run", errUtils.ErrYamlFuncTemplateEmbedding)
	}
	return yamlsource.Replace(input, func(token string, source yamlsource.Source) (string, error) {
		result, err := evaluate(token)
		if err != nil {
			return "", err
		}
		return scalar(result, source)
	})
}

// ResolveResult evaluates functions a result still carries, as when !template renders
// text containing another function's value. A skipped function returns its own source
// unchanged, so that result is left for later processing.
func ResolveResult(result any, skipped func(tag string) bool, evaluate Evaluator) (any, error) {
	defer perf.Track(nil, "embedded.ResolveResult")()

	text, ok := result.(string)
	if !ok {
		return result, nil
	}
	if source, whole := yamlsource.Decode(text); !yamlsource.Embedded(text) && (!whole || skipped(source.Tag)) {
		return result, nil
	}
	return evaluate(text)
}

// IsProvisionalStackName reports a generated stack name built by a name template that
// transforms a function value, as in "{{ upper .vars.stage }}". Atmos replaces these fields
// with the final name once vars resolve, so the interim text isn't a value to evaluate.
func IsProvisionalStackName(key string, value any) bool {
	defer perf.Track(nil, "embedded.IsProvisionalStackName")()

	if key != "atmos_stack" && key != "stack" {
		return false
	}
	text, ok := value.(string)
	return ok && yamlsource.Transformed(text)
}

// scalar renders a function result for insertion into surrounding text.
func scalar(result any, source yamlsource.Source) (string, error) {
	switch value := result.(type) {
	case nil:
		return "", nil
	case string:
		return value, nil
	case bool, int, int32, int64, uint, uint32, uint64, float32, float64:
		return fmt.Sprint(value), nil
	default:
		return "", fmt.Errorf("%w: %q returned %T, and only a single value can be part of a larger string", errUtils.ErrYamlFuncTemplateEmbedding, source.Expression(), result)
	}
}
