package function

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy/admission"
)

// admittedFunction protects registry consumers, including lazy Starlark resolution.
type admittedFunction struct{ Function }

func (f admittedFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.admittedFunction.Execute")()
	if err := admission.Function(ctx, f.Name()); err != nil {
		return nil, err
	}
	return f.Function.Execute(ctx, args, execCtx)
}
