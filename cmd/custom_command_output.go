package cmd

import (
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// customCommandOutputWriter applies explicit output settings while preserving
// the unadorned streaming default of legacy custom-command shell steps.
func customCommandOutputWriter(task *schema.Task, fallbackName string) *stepPkg.OutputModeWriter {
	workflowStep := task.ToWorkflowStep()
	if workflowStep.Name == "" {
		workflowStep.Name = fallbackName
	}
	return stepPkg.NewCommandOutputWriter(&workflowStep, nil)
}
