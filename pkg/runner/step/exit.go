package step

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// ExitHandler exits the workflow with a specific exit code.
type ExitHandler struct {
	BaseHandler
}

func init() {
	Register(&ExitHandler{
		BaseHandler: NewBaseHandler("exit", CategoryUI, false),
	})
}

// Validate checks that the step has valid fields.
func (h *ExitHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.ExitHandler.Validate")()

	// No required fields - code defaults to 0.
	return nil
}

// Execute exits the workflow with the specified exit code.
// It returns an error with the exit code attached that the workflow executor
// should handle to exit the program.
func (h *ExitHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.ExitHandler.Execute")()

	// Display optional message before exiting.
	if step.Content != "" {
		content, err := h.ResolveContent(ctx, step, vars)
		if err != nil {
			return nil, err
		}
		if err := ui.Writeln(content); err != nil {
			return nil, err
		}
	}

	// Return an error with the exit code attached.
	// The workflow executor should check for ErrWorkflowExit and use GetExitCode
	// to determine the exit code.
	code := step.Code
	err := fmt.Errorf("%w: exit code %d", errUtils.ErrWorkflowExit, code)
	return nil, errUtils.WithExitCode(err, code)
}
