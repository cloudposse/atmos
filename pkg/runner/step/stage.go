package step

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// Stage tracking metadata keys.
const (
	MetaStageIndex  = "_stage_index"
	MetaTotalStages = "_total_stages"
)

// StageHandler displays workflow stage position among stage steps.
// Unlike the step count which shows position among all steps,
// stage shows position only among steps of type "stage".
type StageHandler struct {
	BaseHandler
}

func init() {
	Register(&StageHandler{
		BaseHandler: NewBaseHandler("stage", CategoryUI, false),
	})
}

// Validate checks that the step has valid fields.
func (h *StageHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.StageHandler.Validate")()

	// Title is optional (falls back to Name).
	return nil
}

// Execute renders the stage indicator.
// Format: [Stage 1/3] Setup.
func (h *StageHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.StageHandler.Execute")()

	// Increment stage index and get total.
	stageIndex := vars.IncrementStageIndex()
	totalStages := vars.GetTotalStages()

	// Use title if provided, otherwise use step name.
	title := step.Title
	if title == "" {
		title = step.Name
	}

	// Resolve any templates in title.
	resolvedTitle, err := vars.Resolve(title)
	if err != nil {
		return nil, fmt.Errorf("step '%s': failed to resolve title: %w", step.Name, err)
	}

	// Format: [Stage 1/3] Setup
	output := formatStageOutput(stageIndex, totalStages, resolvedTitle)
	ui.Writeln(output)

	return NewStepResult(resolvedTitle), nil
}

// formatStageOutput creates the styled stage output.
func formatStageOutput(index, total int, title string) string {
	styles := theme.GetCurrentStyles()

	prefix := fmt.Sprintf("[Stage %d/%d]", index, total)

	if styles != nil {
		return styles.Label.Render(prefix) + " " + styles.Title.Render(title)
	}
	return fmt.Sprintf("%s %s", prefix, title)
}

// CountStages counts the number of stage steps in a workflow.
func CountStages(workflow *schema.WorkflowDefinition) int {
	defer perf.Track(nil, "step.CountStages")()

	count := 0
	for i := range workflow.Steps {
		if workflow.Steps[i].Type == "stage" {
			count++
		}
	}
	return count
}
