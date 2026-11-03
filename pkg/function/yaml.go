package function

import (
	"context"
	"strings"
	"unicode"

	"github.com/cloudposse/atmos/pkg/function/tag"
	"github.com/cloudposse/atmos/pkg/perf"
)

// deferredYAMLFunctions opts functions into registry-backed YAML evaluation.
// Legacy functions retain their host evaluators until explicitly migrated here.
var deferredYAMLFunctions = map[string]struct{}{
	TagCILabels: {},
}

// EvaluateDeferredYAML evaluates an opted-in function through the registry.
// Unknown, legacy, and skipped expressions are preserved without evaluation.
func EvaluateDeferredYAML(ctx context.Context, input string, skip []string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.EvaluateDeferredYAML")()

	name, tagged := strings.CutPrefix(input, "!")
	if !tagged {
		return input, nil
	}
	args := ""
	if boundary := strings.IndexFunc(name, unicode.IsSpace); boundary >= 0 {
		name, args = name[:boundary], strings.TrimSpace(name[boundary:])
	}
	if _, enabled := deferredYAMLFunctions[name]; !enabled || tag.IsSkipped(skip, name) {
		return input, nil
	}
	fn, err := DefaultRegistry().Get(name)
	if err != nil {
		return nil, err
	}
	return fn.Execute(ctx, args, execCtx)
}
