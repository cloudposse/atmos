package function

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Function defines the interface for all Atmos configuration functions.
// Functions are format-agnostic and can be used in YAML, HCL, or JSON configurations.
type Function interface {
	// Name returns the primary function name (e.g., "env", "terraform.output").
	Name() string

	// Aliases returns alternative names for the function.
	Aliases() []string

	// Phase returns when this function should be executed.
	Phase() Phase

	// Execute processes the function with the given arguments and context.
	Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error)
}

// BaseFunction provides a reusable implementation of the Function interface.
// Embed this struct in concrete function types to inherit common behavior.
type BaseFunction struct {
	FunctionName    string
	FunctionAliases []string
	FunctionPhase   Phase
}

// Name returns the primary function name.
func (f *BaseFunction) Name() string {
	defer perf.Track(nil, "function.BaseFunction.Name")()

	return f.FunctionName
}

// Aliases returns alternative names for the function.
func (f *BaseFunction) Aliases() []string {
	defer perf.Track(nil, "function.BaseFunction.Aliases")()

	return f.FunctionAliases
}

// Phase returns when this function should be executed.
func (f *BaseFunction) Phase() Phase {
	defer perf.Track(nil, "function.BaseFunction.Phase")()

	return f.FunctionPhase
}
