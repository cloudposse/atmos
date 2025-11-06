package workflow

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// WorkflowOptions provides strongly-typed access to workflow command flags.
// Embeds StandardOptions and adds workflow-specific flags.
//
// Example usage:
//
//	// Type-safe access to positional args (populated automatically by parser):
//	fmt.Printf("Executing workflow: %s\n", opts.WorkflowName)
//
// See docs/prd/flag-handling/type-safe-positional-arguments.md for patterns.
type WorkflowOptions struct {
	flags.StandardOptions // Embedded standard flags (stack, file, dry-run, identity, etc.)

	// Positional arguments (populated automatically by parser from TargetField mapping).
	WorkflowName string // Workflow name from positional arg (e.g., "deploy" in: atmos workflow deploy)

	// FromStep specifies the step to resume workflow execution from.
	FromStep string
}

// GetGlobalFlags returns a pointer to the embedded GlobalFlags.
// Implements CommandOptions interface.
func (w *WorkflowOptions) GetGlobalFlags() *flags.GlobalFlags {
	defer perf.Track(nil, "flags.WorkflowOptions.GetGlobalFlags")()

	return w.StandardOptions.GetGlobalFlags()
}

// GetPositionalArgs returns positional arguments extracted by the parser.
// For workflow command: workflow name.
func (w *WorkflowOptions) GetPositionalArgs() []string {
	defer perf.Track(nil, "flags.WorkflowOptions.GetPositionalArgs")()

	return w.StandardOptions.GetPositionalArgs()
}

// GetSeparatedArgs returns pass-through arguments.
// For workflow command: always empty (no pass-through).
func (w *WorkflowOptions) GetSeparatedArgs() []string {
	defer perf.Track(nil, "flags.WorkflowOptions.GetSeparatedArgs")()

	return w.StandardOptions.GetSeparatedArgs()
}
