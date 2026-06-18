package step

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
)

type executionOptionsKey struct{}

// ExecutionOptions carries execution settings that are not part of workflow schema.
type ExecutionOptions struct {
	DryRun             bool
	AtmosStackOverride string
}

// WithExecutionOptions attaches execution settings to ctx.
func WithExecutionOptions(ctx context.Context, opts ExecutionOptions) context.Context {
	defer perf.Track(nil, "step.WithExecutionOptions")()

	return context.WithValue(ctx, executionOptionsKey{}, opts)
}

func executionOptionsFromContext(ctx context.Context) ExecutionOptions {
	if ctx == nil {
		return ExecutionOptions{}
	}
	opts, _ := ctx.Value(executionOptionsKey{}).(ExecutionOptions)
	return opts
}
