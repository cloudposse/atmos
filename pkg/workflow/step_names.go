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
// nested steps), recursing into nested step lists. Generated names never
// collide with an explicit sibling name, since step results are keyed by name.
func generateWorkflowStepNames(steps []schema.WorkflowStep, parent string) {
	used := make(map[string]bool, len(steps))
	for i := range steps {
		if steps[i].Name != "" {
			used[steps[i].Name] = true
		}
	}
	for index := range steps {
		step := &steps[index]
		if step.Name == "" {
			candidate := fmt.Sprintf("step%d", index+1)
			if parent != "" {
				candidate = fmt.Sprintf("%s_step%d", parent, index+1)
			}
			for used[candidate] {
				candidate += "_"
			}
			step.Name = candidate
			used[candidate] = true
		}
		if len(step.Steps) > 0 {
			generateWorkflowStepNames(step.Steps, step.Name)
		}
	}
}
