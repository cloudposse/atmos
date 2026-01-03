package step

import (
	"context"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// LinebreakHandler outputs one or more blank lines.
type LinebreakHandler struct {
	BaseHandler
}

func init() {
	Register(&LinebreakHandler{
		BaseHandler: NewBaseHandler("linebreak", CategoryUI, false),
	})
}

// Validate checks that the step has valid fields.
func (h *LinebreakHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.LinebreakHandler.Validate")()

	// No required fields - count defaults to 1 if not specified.
	return nil
}

// Execute outputs the specified number of blank lines.
func (h *LinebreakHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.LinebreakHandler.Execute")()

	count := step.Count
	if count <= 0 {
		count = 1
	}

	output := strings.Repeat("\n", count)
	if err := ui.Write(output); err != nil {
		return nil, err
	}

	return NewStepResult(""), nil
}
