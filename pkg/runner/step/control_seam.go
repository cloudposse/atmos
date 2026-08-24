package step

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ControlRunner executes control-flow steps (parallel/matrix). These step types
// fan out to child steps and need the scheduler/dependency graph plus, in a
// workflow, the run-scoped auth and output aggregation — machinery that lives in
// pkg/workflow, which pkg/runner/step cannot import without a cycle (the import
// direction is pkg/workflow -> pkg/runner/step). So the implementation is
// registered here at startup by pkg/workflow via RegisterControlRunner, the same
// reverse-dependency seam the emulator step uses (see emulator.go). When no
// runner is registered (e.g. a pure pkg/runner/step unit test that does not link
// pkg/workflow) the control handlers report that they require the workflow
// executor context.
type ControlRunner interface {
	// RunControl fans out a parallel/matrix step to its children and blocks
	// until they complete (honoring fail policy and output aggregation).
	RunControl(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error)
}

// controlRunner holds the registered ControlRunner implementation.
var controlRunner ControlRunner

// RegisterControlRunner wires the control-step implementation. It is called from
// pkg/workflow's init so parallel/matrix steps work whenever that package is
// linked into the binary (always, for the CLI). The seam is what lets custom
// commands and lifecycle hooks run parallel/matrix steps through the registry —
// not just `atmos workflow`.
func RegisterControlRunner(r ControlRunner) {
	defer perf.Track(nil, "step.RegisterControlRunner")()

	controlRunner = r
}

// runControlStep dispatches a parallel/matrix step to the registered runner, or
// returns the "requires workflow executor context" error when none is linked.
func runControlStep(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	if controlRunner == nil {
		return nil, fmt.Errorf("%w: %s steps require workflow executor context", schema.ErrWorkflowControlStepInvalid, step.Type)
	}
	return controlRunner.RunControl(ctx, step, vars)
}

// validateControlChildrenNonInteractive rejects interactive (TTY-requiring) child
// steps inside a parallel/matrix step: interactive prompts cannot run
// concurrently. The RequiresTTY() flag on the child's registered handler is the
// signal (e.g. input, choose, confirm, pager).
func validateControlChildrenNonInteractive(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.validateControlChildrenNonInteractive")()

	for i := range step.Steps {
		child := &step.Steps[i]
		childType := child.Type
		if childType == "" {
			childType = schema.TaskTypeShell
		}
		if handler, ok := Get(childType); ok && handler.RequiresTTY() {
			return fmt.Errorf("%w: %s step %q has interactive child %q (type %q); interactive steps cannot run inside a %s step",
				schema.ErrWorkflowControlStepInvalid, step.Type, step.Name, child.Name, childType, step.Type)
		}
	}
	return nil
}
