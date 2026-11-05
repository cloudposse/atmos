package function

import (
	"context"
	"slices"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/function/parser"
	"github.com/cloudposse/atmos/pkg/perf"
)

// CILabelsFunction returns CI review labels or tests whether all requested labels exist.
type CILabelsFunction struct{ BaseFunction }

// NewCILabelsFunction creates a post-merge CI label function.
func NewCILabelsFunction() *CILabelsFunction {
	defer perf.Track(nil, "function.NewCILabelsFunction")()

	return &CILabelsFunction{BaseFunction{FunctionName: TagCILabels, FunctionPhase: PostMerge}}
}

// Execute returns a merge-compatible list or a native boolean for membership matching.
func (f *CILabelsFunction) Execute(ctx context.Context, args string, _ *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.CILabelsFunction.Execute")()

	requested, err := parser.ParseCILabels(args)
	if err != nil {
		return nil, err
	}
	labels, err := ci.Labels(ctx)
	if err != nil {
		return nil, err
	}
	if requested != nil {
		for _, label := range requested {
			if !slices.Contains(labels, label) {
				return false, nil
			}
		}
		return true, nil
	}
	result := make([]any, len(labels))
	for i, label := range labels {
		result[i] = label
	}
	return result, nil
}
