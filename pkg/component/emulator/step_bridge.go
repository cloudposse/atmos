package emulator

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// init wires the emulator lifecycle into the workflow step registry so the
// `emulator` step type (used by `kind: step` lifecycle hooks) can start and stop
// a sandbox. The runner lives here — not in pkg/runner/step — because the step
// package cannot import this package without an import cycle through
// internal/exec. The dependency direction is reversed: the implementer
// (pkg/component/emulator) depends on the registry seam in pkg/runner/step.
func init() {
	step.RegisterEmulatorRunner(stepRunner{})
}

// stepRunner adapts the component-emulator executors to the step.EmulatorRunner
// seam, building the minimal ConfigAndStacksInfo each action needs from the
// component name and stack supplied by the step.
type stepRunner struct{}

func (stepRunner) Up(ctx context.Context, component, stack string, ephemeral bool) error {
	defer perf.Track(nil, "emulator.stepRunner.Up")()

	info := stepInfo(component, stack)
	if ephemeral {
		return ExecuteUpEphemeral(ctx, info)
	}
	return ExecuteUp(ctx, info)
}

func (stepRunner) Down(ctx context.Context, component, stack string) error {
	defer perf.Track(nil, "emulator.stepRunner.Down")()

	return ExecuteDown(ctx, stepInfo(component, stack))
}

func (stepRunner) Reset(ctx context.Context, component, stack string) error {
	defer perf.Track(nil, "emulator.stepRunner.Reset")()

	// Step-driven resets are non-interactive (force=true): there is no operator
	// prompt mid-hook.
	return ExecuteReset(ctx, stepInfo(component, stack), true)
}

// stepInfo builds the ConfigAndStacksInfo the executors resolve from.
func stepInfo(component, stack string) *schema.ConfigAndStacksInfo {
	return &schema.ConfigAndStacksInfo{
		ComponentFromArg: component,
		Component:        component,
		Stack:            stack,
	}
}
