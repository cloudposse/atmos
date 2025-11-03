package flags

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// WorkflowOptions provides strongly-typed access to workflow command flags.
// Embeds StandardOptions and adds workflow-specific flags.
type WorkflowOptions struct {
	StandardOptions // Embedded standard flags (stack, file, dry-run, identity, etc.)

	// FromStep specifies the step to resume workflow execution from.
	FromStep string
}

// GetGlobalFlags returns a pointer to the embedded GlobalFlags.
// Implements CommandOptions interface.
func (w *WorkflowOptions) GetGlobalFlags() *GlobalFlags {
	defer perf.Track(nil, "flags.WorkflowOptions.GetGlobalFlags")()

	return w.StandardOptions.GetGlobalFlags()
}

// GetPositionalArgs returns positional arguments extracted by the parser.
// For workflow command: workflow name.
func (w *WorkflowOptions) GetPositionalArgs() []string {
	defer perf.Track(nil, "flags.WorkflowOptions.GetPositionalArgs")()

	return w.StandardOptions.GetPositionalArgs()
}

// GetPassThroughArgs returns pass-through arguments.
// For workflow command: always empty (no pass-through).
func (w *WorkflowOptions) GetPassThroughArgs() []string {
	defer perf.Track(nil, "flags.WorkflowOptions.GetPassThroughArgs")()

	return w.StandardOptions.GetPassThroughArgs()
}
