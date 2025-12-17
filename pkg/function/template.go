package function

import (
	"context"
	"encoding/json"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// TemplateFunction implements the template function for JSON template processing.
type TemplateFunction struct {
	BaseFunction
}

// NewTemplateFunction creates a new template function handler.
func NewTemplateFunction() *TemplateFunction {
	defer perf.Track(nil, "function.NewTemplateFunction")()

	return &TemplateFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagTemplate,
			FunctionAliases: nil,
			FunctionPhase:   PreMerge,
		},
	}
}

// Execute processes the template function.
// Usage:
//
//	!template {"key": "value"}   - Parse JSON and return as native type
//	!template [1, 2, 3]          - Parse JSON array and return as native slice
//
// If the input is valid JSON, it will be parsed and returned as the corresponding type.
// Otherwise, the raw string is returned.
func (f *TemplateFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.TemplateFunction.Execute")()

	log.Debug("Executing template function", "args", args)

	args = strings.TrimSpace(args)
	if args == "" {
		return "", nil
	}

	// Try to parse as JSON.
	var decoded any
	if err := json.Unmarshal([]byte(args), &decoded); err != nil {
		// Not valid JSON, return as-is.
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

	templatePrefix := YAMLTag(TagTemplate)
	result := make(map[string]any, len(input))

	var recurse func(any) any
	recurse = func(node any) any {
		switch v := node.(type) {
		case string:
			// Only process !template tags, leave other tags as-is.
			if strings.HasPrefix(v, templatePrefix) {
				// Extract args after the tag.
				args := strings.TrimPrefix(v, templatePrefix)
				args = strings.TrimSpace(args)

				// Parse as JSON if possible.
				var decoded any
				if err := json.Unmarshal([]byte(args), &decoded); err != nil {
					return args
				}
				return decoded
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
