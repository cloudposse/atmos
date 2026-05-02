package ci

import (
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecuteOptions contains options for executing CI hooks.
type ExecuteOptions struct {
	// Event is the hook event (e.g., "after.terraform.plan").
	Event string

	// AtmosConfig is the Atmos configuration.
	AtmosConfig *schema.AtmosConfiguration

	// Info contains component and stack information.
	Info *schema.ConfigAndStacksInfo

	// Output is the command output to process.
	Output string

	// ComponentType overrides the component type detection.
	// If empty, it's extracted from the event.
	ComponentType string

	// ForceCIMode forces CI mode even if environment detection fails.
	// This is set when --ci flag is used.
	ForceCIMode bool

	// CommandError is the error from the command execution, if any.
	// When set, check runs are updated with failure status.
	CommandError error
}

// executeFunc is set by the executor package at init time to break the import cycle.
// The executor package imports pkg/ci (for registry access) so pkg/ci cannot import it.
var executeFunc func(*ExecuteOptions) error

// RegisterExecutor is called by the executor package to register its Execute implementation.
// This breaks the import cycle: executor imports ci, and ci calls executor via this function pointer.
func RegisterExecutor(fn func(*ExecuteOptions) error) {
	defer perf.Track(nil, "ci.RegisterExecutor")()

	executeFunc = fn
}

// Execute runs all CI actions for a hook event.
// Returns nil if not in CI or if the event is not handled.
func Execute(opts *ExecuteOptions) error {
	defer perf.Track(nil, "ci.Execute")()

	if opts == nil {
		return errUtils.ErrCIOptionsRequired
	}
	if executeFunc == nil {
		return errUtils.ErrCIExecutorNotWired
	}
	return executeFunc(opts)
}
