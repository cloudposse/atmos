package function

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// TagTemplate is the YAML tag for the template function.
const TagTemplate = "!template"

// TemplateFunction implements the !template YAML function.
// It parses JSON content embedded in YAML strings.
type TemplateFunction struct {
	BaseFunction
}

// NewTemplateFunction creates a new TemplateFunction.
func NewTemplateFunction() *TemplateFunction {
	defer perf.Track(nil, "function.NewTemplateFunction")()

	return &TemplateFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "template",
			FunctionAliases: nil,
			FunctionPhase:   PreMerge,
		},
	}
}

// Execute processes the !template function.
// Syntax: !template {"json": "content"}
// Returns the parsed JSON as a Go value, or the raw string if not valid JSON.
func (f *TemplateFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.TemplateFunction.Execute")()

	args = strings.TrimSpace(args)
	if args == "" {
		return "", nil
	}

	// Try to parse as JSON.
	var decoded any
	if err := json.Unmarshal([]byte(args), &decoded); err != nil {
		// If not valid JSON, return the raw string.
		return args, nil
	}

	return decoded, nil
}

// ProcessTemplateTagsOnly processes only !template tags in a data structure, recursively.
// It is used before merging to ensure !template strings are decoded to their actual types.
// This avoids type conflicts during merge (e.g., string vs list).
func ProcessTemplateTagsOnly(input map[string]any) map[string]any {
	defer perf.Track(nil, "function.ProcessTemplateTagsOnly")()

	if input == nil {
		return nil
	}

	result := make(map[string]any, len(input))
	templateFn := NewTemplateFunction()

	var recurse func(any) any
	recurse = func(node any) any {
		switch v := node.(type) {
		case string:
			// Only process !template tags, leave other tags as-is.
			if strings.HasPrefix(v, TagTemplate) {
				args := strings.TrimPrefix(v, TagTemplate)
				result, _ := templateFn.Execute(context.Background(), args, nil)
				return result
			}
			return v

		case map[string]any:
			newMap := make(map[string]any, len(v))
			for k, val := range v {
				newMap[k] = recurse(val)
			}
			return newMap

		case []any:
			newSlice := make([]any, len(v))
			for i, val := range v {
				newSlice[i] = recurse(val)
			}
			return newSlice

		default:
			return v
		}
	}

	for k, v := range input {
		result[k] = recurse(v)
	}

	return result
}
