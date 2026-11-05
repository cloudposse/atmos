package function

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
)

// StarlarkFunction computes a value using the consuming component's context.
type StarlarkFunction struct{ BaseFunction }

// NewStarlarkFunction registers evaluation after component configuration merges.
func NewStarlarkFunction() *StarlarkFunction {
	defer perf.Track(nil, "function.NewStarlarkFunction")()

	return &StarlarkFunction{BaseFunction{FunctionName: TagStarlark, FunctionPhase: PostMerge}}
}

// Execute delegates to the invocation's configuration evaluator.
func (f *StarlarkFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.StarlarkFunction.Execute")()

	if execCtx == nil || execCtx.EvaluateValue == nil {
		return nil, fmt.Errorf("%w: !starlark requires a merged stack/component configuration context", ErrInvalidArguments)
	}
	return execCtx.EvaluateValue(ctx, args)
}
