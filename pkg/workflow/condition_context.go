package workflow

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

// BuildConditionContext constructs the runtime facts exposed to workflow `when`
// conditions. Step stack overrides workflow stack, command-line stack overrides
// both, and step env overlays workflow/base env.
func BuildConditionContext(workflow string, workflowDefinition *schema.WorkflowDefinition, step *schema.WorkflowStep, commandLineStack string, baseEnv map[string]string) schema.ConditionContext {
	defer perf.Track(nil, "workflow.BuildConditionContext")()

	stack := ""
	stepName := ""
	env := baseEnv
	if workflowDefinition != nil {
		stack = workflowDefinition.Stack
		if env == nil {
			env = workflowDefinition.Env
		}
	}
	if step != nil {
		if step.Stack != "" {
			stack = step.Stack
		}
		stepName = step.Name
		if len(step.Env) > 0 {
			merged := make(map[string]string, len(env))
			for key, value := range env {
				merged[key] = value
			}
			for key, value := range step.Env {
				merged[key] = value
			}
			env = merged
		}
	}
	if commandLineStack != "" {
		stack = commandLineStack
	}
	return schema.ConditionContext{
		CI:       telemetry.IsCI(),
		Status:   schema.ConditionPredicateSuccess,
		Stack:    stack,
		Workflow: workflow,
		Step:     stepName,
		Env:      env,
	}
}
