package function

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecutionContext provides the runtime context for function execution.
// It contains all the information a function might need to resolve values.
type ExecutionContext struct {
	// AtmosConfig is the current Atmos configuration.
	AtmosConfig *schema.AtmosConfiguration

	// Stack is the current stack name being processed.
	Stack string

	// Component is the current component name being processed.
	Component string

	// BaseDir is the base directory for relative path resolution.
	BaseDir string

	// File is the path to the file being processed.
	File string

	// StackInfo contains additional stack and component information.
	StackInfo *schema.ConfigAndStacksInfo
}

// NewExecutionContext creates a new ExecutionContext with the given parameters.
func NewExecutionContext(atmosConfig *schema.AtmosConfiguration, stack, component string) *ExecutionContext {
	defer perf.Track(atmosConfig, "function.NewExecutionContext")()

	return &ExecutionContext{
		AtmosConfig: atmosConfig,
		Stack:       stack,
		Component:   component,
	}
}

// WithFile returns a copy of the context with the file path set.
func (ctx *ExecutionContext) WithFile(file string) *ExecutionContext {
	newCtx := *ctx
	newCtx.File = file
	return &newCtx
}

// WithBaseDir returns a copy of the context with the base directory set.
func (ctx *ExecutionContext) WithBaseDir(baseDir string) *ExecutionContext {
	newCtx := *ctx
	newCtx.BaseDir = baseDir
	return &newCtx
}

// WithStackInfo returns a copy of the context with stack info set.
func (ctx *ExecutionContext) WithStackInfo(stackInfo *schema.ConfigAndStacksInfo) *ExecutionContext {
	newCtx := *ctx
	newCtx.StackInfo = stackInfo
	return &newCtx
}
