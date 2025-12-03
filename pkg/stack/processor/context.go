package processor

import (
	"github.com/cloudposse/atmos/pkg/function"
	"github.com/cloudposse/atmos/pkg/perf"
)

// StackContext extends function.ExecutionContext with stack-specific fields.
// This provides the complete context needed for post-merge function processing.
type StackContext struct {
	// ExecutionContext contains the base execution environment.
	*function.ExecutionContext

	// CurrentStack is the name of the stack being processed.
	CurrentStack string

	// CurrentComponent is the name of the component being processed.
	CurrentComponent string

	// Skip is a list of function names to skip during processing.
	// This allows for selective function execution.
	Skip []string

	// DryRun indicates whether this is a dry run (no side effects).
	DryRun bool

	// StacksBasePath is the base path for stack configuration files.
	StacksBasePath string

	// ComponentsBasePath is the base path for component configurations.
	ComponentsBasePath string
}

// NewStackContext creates a new stack context with the given parameters.
func NewStackContext(execCtx *function.ExecutionContext) *StackContext {
	defer perf.Track(nil, "processor.NewStackContext")()

	if execCtx == nil {
		execCtx = function.NewExecutionContext(nil, "", "")
	}

	return &StackContext{
		ExecutionContext: execCtx,
		Skip:             make([]string, 0),
	}
}

// WithStack sets the current stack name.
func (c *StackContext) WithStack(stack string) *StackContext {
	defer perf.Track(nil, "processor.StackContext.WithStack")()

	c.CurrentStack = stack
	return c
}

// WithComponent sets the current component name.
func (c *StackContext) WithComponent(component string) *StackContext {
	defer perf.Track(nil, "processor.StackContext.WithComponent")()

	c.CurrentComponent = component
	return c
}

// WithSkip sets the list of functions to skip.
func (c *StackContext) WithSkip(skip []string) *StackContext {
	defer perf.Track(nil, "processor.StackContext.WithSkip")()

	if skip == nil {
		c.Skip = make([]string, 0)
	} else {
		c.Skip = skip
	}
	return c
}

// WithDryRun sets the dry run flag.
func (c *StackContext) WithDryRun(dryRun bool) *StackContext {
	defer perf.Track(nil, "processor.StackContext.WithDryRun")()

	c.DryRun = dryRun
	return c
}

// WithStacksBasePath sets the stacks base path.
func (c *StackContext) WithStacksBasePath(path string) *StackContext {
	defer perf.Track(nil, "processor.StackContext.WithStacksBasePath")()

	c.StacksBasePath = path
	return c
}

// WithComponentsBasePath sets the components base path.
func (c *StackContext) WithComponentsBasePath(path string) *StackContext {
	defer perf.Track(nil, "processor.StackContext.WithComponentsBasePath")()

	c.ComponentsBasePath = path
	return c
}

// ShouldSkip returns true if the given function name should be skipped.
func (c *StackContext) ShouldSkip(functionName string) bool {
	defer perf.Track(nil, "processor.StackContext.ShouldSkip")()

	if c == nil || len(c.Skip) == 0 {
		return false
	}

	for _, skip := range c.Skip {
		if skip == functionName {
			return true
		}
	}
	return false
}

// Clone creates a shallow copy of the context.
func (c *StackContext) Clone() *StackContext {
	defer perf.Track(nil, "processor.StackContext.Clone")()

	if c == nil {
		return nil
	}

	skipCopy := make([]string, len(c.Skip))
	copy(skipCopy, c.Skip)

	return &StackContext{
		ExecutionContext:   c.ExecutionContext,
		CurrentStack:       c.CurrentStack,
		CurrentComponent:   c.CurrentComponent,
		Skip:               skipCopy,
		DryRun:             c.DryRun,
		StacksBasePath:     c.StacksBasePath,
		ComponentsBasePath: c.ComponentsBasePath,
	}
}
