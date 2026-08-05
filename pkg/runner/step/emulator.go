package step

import (
	"context"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	emulatorStepType    = "emulator"
	emulatorActionUp    = "up"
	emulatorActionDown  = "down"
	emulatorActionReset = "reset"
)

// EmulatorRunner drives the lifecycle of an emulator component. It is the seam
// that lets the emulator step start/stop a sandbox without pkg/runner/step
// importing pkg/component/emulator directly (which would create an import cycle
// through internal/exec). The implementation is registered at startup via
// RegisterEmulatorRunner by pkg/component/emulator.
type EmulatorRunner interface {
	// Up starts (or reuses) the emulator component's container in the stack.
	// When ephemeral is true the instance runs without persistence.
	Up(ctx context.Context, component, stack string, ephemeral bool) error
	// Down stops and removes the emulator component's container.
	Down(ctx context.Context, component, stack string) error
	// Reset stops the container and wipes its persisted state.
	Reset(ctx context.Context, component, stack string) error
}

// emulatorRunner holds the registered EmulatorRunner implementation.
var emulatorRunner EmulatorRunner

// RegisterEmulatorRunner wires the emulator lifecycle implementation. It is
// called from pkg/component/emulator's init so the `emulator` step works
// whenever that package is linked into the binary (always, via cmd/emulator).
func RegisterEmulatorRunner(r EmulatorRunner) {
	defer perf.Track(nil, "step.RegisterEmulatorRunner")()

	emulatorRunner = r
}

// EmulatorHandler manages an emulator component's lifecycle as a step, so
// component lifecycle hooks (`kind: step`, `type: emulator`) can bring a local
// cloud sandbox up before a terraform operation and tear it down after.
type EmulatorHandler struct {
	BaseHandler
}

func init() {
	Register(&EmulatorHandler{
		BaseHandler: NewBaseHandler(emulatorStepType, CategoryCommand, false),
	})
}

// emulatorStepAction returns the step action, defaulting to "up".
func emulatorStepAction(step *schema.WorkflowStep) string {
	action := strings.TrimSpace(step.Action)
	if action == "" {
		return emulatorActionUp
	}
	return action
}

// Validate checks emulator step configuration.
func (h *EmulatorHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.EmulatorHandler.Validate")()

	if strings.TrimSpace(step.Component) == "" {
		return errUtils.Build(errUtils.ErrStepFieldRequired).
			WithContext("step", step.Name).
			WithContext("field", "component").
			WithExplanation("An emulator step must set `component` to the emulator component name (e.g. component: aws)").
			Err()
	}

	switch emulatorStepAction(step) {
	case emulatorActionUp, emulatorActionDown, emulatorActionReset:
		return nil
	default:
		return errUtils.Build(errUtils.ErrStepFieldRequired).
			WithContext("step", step.Name).
			WithContext("field", "action").
			WithContext("value", step.Action).
			WithExplanation("Action must be `up`, `down`, or `reset`").
			Err()
	}
}

// Execute runs the emulator lifecycle action.
func (h *EmulatorHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.EmulatorHandler.Execute")()

	if err := h.Validate(step); err != nil {
		return nil, err
	}

	if emulatorRunner == nil {
		return nil, errUtils.Build(errUtils.ErrStepExecutionFailed).
			WithContext("step", step.Name).
			WithContext("type", emulatorStepType).
			WithExplanation("The emulator step runner is not registered").
			WithHint("This is an internal wiring error; ensure pkg/component/emulator is linked into the binary").
			Err()
	}

	component, err := vars.Resolve(step.Component)
	if err != nil {
		return nil, fmt.Errorf("step '%s': failed to resolve component: %w", step.Name, err)
	}

	// The stack defaults to the component's stack (ATMOS_STACK, injected by the
	// hook engine) so an emulator hook needs no explicit stack; an explicit
	// `stack:` in the step overrides it.
	stack := strings.TrimSpace(step.Stack)
	if stack == "" {
		stack = vars.Env["ATMOS_STACK"]
	} else if stack, err = vars.Resolve(stack); err != nil {
		return nil, fmt.Errorf("step '%s': failed to resolve stack: %w", step.Name, err)
	}

	action := emulatorStepAction(step)
	switch action {
	case emulatorActionUp:
		err = emulatorRunner.Up(ctx, component, stack, step.Ephemeral)
	case emulatorActionDown:
		err = emulatorRunner.Down(ctx, component, stack)
	case emulatorActionReset:
		err = emulatorRunner.Reset(ctx, component, stack)
	}
	if err != nil {
		return nil, fmt.Errorf("step '%s': emulator %s %q failed: %w", step.Name, action, component, err)
	}

	return NewStepResult(fmt.Sprintf("emulator %s %s", component, action)), nil
}
