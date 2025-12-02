package function

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Function defines the interface for a configuration function.
// Functions process special syntax in configuration files (e.g., !env, !terraform.output).
type Function interface {
	// Name returns the primary function name (e.g., "env", "terraform.output").
	Name() string

	// Aliases returns alternative names for this function.
	// For example, "store" might be an alias for "store.get".
	Aliases() []string

	// Phase returns when this function should be executed.
	// PreMerge functions run during initial file loading.
	// PostMerge functions run after configuration merging.
	Phase() Phase

	// Execute runs the function with the given arguments.
	// The args string contains everything after the function name in the syntax.
	// Returns the processed value or an error.
	Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error)
}

// BaseFunction provides a base implementation for Function that can be embedded.
type BaseFunction struct {
	FunctionName    string
	FunctionAliases []string
	FunctionPhase   Phase
}

// Name returns the function name.
func (f *BaseFunction) Name() string {
	defer perf.Track(nil, "function.BaseFunction.Name")()

	return f.FunctionName
}

// Aliases returns the function aliases.
func (f *BaseFunction) Aliases() []string {
	defer perf.Track(nil, "function.BaseFunction.Aliases")()

	if f.FunctionAliases == nil {
		return []string{}
	}
	return f.FunctionAliases
}

// Phase returns the function phase.
func (f *BaseFunction) Phase() Phase {
	defer perf.Track(nil, "function.BaseFunction.Phase")()

	return f.FunctionPhase
}
