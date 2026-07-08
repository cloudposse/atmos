package workflow

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/schema"
)

// CheckAndGenerateWorkflowStepNames assigns generated names to any workflow steps
// (and nested steps) that do not declare one, so every step is addressable.
func CheckAndGenerateWorkflowStepNames(workflowDefinition *schema.WorkflowDefinition) {
	generateWorkflowStepNames(workflowDefinition.Steps, "")
}

// generateWorkflowStepNames names unnamed steps as stepN (or parent_stepN for
// nested steps), recursing into nested step lists.
func generateWorkflowStepNames(steps []schema.WorkflowStep, parent string) {
	for index := range steps {
		step := &steps[index]
		if step.Name == "" {
			if parent == "" {
				step.Name = fmt.Sprintf("step%d", index+1)
			} else {
				step.Name = fmt.Sprintf("%s_step%d", parent, index+1)
			}
		}
		if len(step.Steps) > 0 {
			generateWorkflowStepNames(step.Steps, step.Name)
		}
	}
}
