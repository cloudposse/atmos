package function

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecutionContext provides the minimal context needed for function execution.
// This contains basic execution environment information.
// Stack-specific context is provided via pkg/stack/processor.
type ExecutionContext struct {
	// Env contains environment variables available for function execution.
	Env map[string]string

	// WorkingDir is the current working directory.
	WorkingDir string

	// SourceFile is the file containing this function call.
	SourceFile string

	// Stack is the current stack name being processed.
	Stack string

	// AtmosConfig is the Atmos configuration.
	AtmosConfig *schema.AtmosConfiguration

	// StackInfo contains stack processing information including auth context.
	StackInfo *StackInfo
}

// StackInfo contains information about the stack being processed.
type StackInfo struct {
	// AuthContext contains authentication context for cloud providers.
	AuthContext *schema.AuthContext
}

// NewExecutionContext creates a new execution context with the given parameters.
func NewExecutionContext(env map[string]string, workingDir, sourceFile string) *ExecutionContext {
	defer perf.Track(nil, "function.NewExecutionContext")()

	if env == nil {
		env = make(map[string]string)
	}
	return &ExecutionContext{
		Env:        env,
		WorkingDir: workingDir,
		SourceFile: sourceFile,
	}
}

// GetEnv returns the value of an environment variable, or empty string if not found.
func (c *ExecutionContext) GetEnv(key string) string {
	defer perf.Track(nil, "function.ExecutionContext.GetEnv")()

	if c == nil || c.Env == nil {
		return ""
	}
	return c.Env[key]
}

// HasEnv returns true if the environment variable is set.
func (c *ExecutionContext) HasEnv(key string) bool {
	defer perf.Track(nil, "function.ExecutionContext.HasEnv")()

	if c == nil || c.Env == nil {
		return false
	}
	_, ok := c.Env[key]
	return ok
}
