package exec

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// resolveWorkflowStepFunctions evaluates value-producing YAML functions (!env,
// !exec) in the interactive fields of workflow steps.
//
// Workflow manifests are parsed with utils.UnmarshalYAML, which stringifies
// custom YAML tags: `default: !env FOO bar` becomes the literal string
// "!env FOO bar" for later evaluation. Stacks evaluate these during stack
// processing, but workflows have no such phase — so without this a step's
// `default`/`prompt`/`placeholder`/`options` would keep the literal "!env ..."
// text. This resolves the context-free functions (!env, !exec) so interactive
// steps can source their defaults from the environment in CI.
//
// Stack-dependent functions (e.g. !terraform.output, !store, !secret) are
// intentionally left unevaluated because a workflow step has no component/stack
// resolution context; they pass through unchanged.
func resolveWorkflowStepFunctions(atmosConfig *schema.AtmosConfiguration, def *schema.WorkflowDefinition) error {
	defer perf.Track(atmosConfig, "exec.resolveWorkflowStepFunctions")()

	if def == nil {
		return nil
	}
	return resolveWorkflowStepsFunctions(atmosConfig, def.Steps)
}

// resolveWorkflowStepsFunctions resolves step-field functions for a list of
// steps, recursing into nested steps (parallel/matrix/cast children).
func resolveWorkflowStepsFunctions(atmosConfig *schema.AtmosConfiguration, steps []schema.WorkflowStep) error {
	for i := range steps {
		if err := resolveWorkflowStepFieldFunctions(atmosConfig, &steps[i]); err != nil {
			return err
		}
		if len(steps[i].Steps) > 0 {
			if err := resolveWorkflowStepsFunctions(atmosConfig, steps[i].Steps); err != nil {
				return err
			}
		}
	}
	return nil
}

// resolveWorkflowStepFieldFunctions resolves supported YAML functions in the
// interactive scalar fields of a single step.
func resolveWorkflowStepFieldFunctions(atmosConfig *schema.AtmosConfiguration, step *schema.WorkflowStep) error {
	resolved, err := resolveStepFunctionString(atmosConfig, step.Default)
	if err != nil {
		return workflowStepFunctionError(step, "default", err)
	}
	step.Default = resolved

	resolved, err = resolveStepFunctionString(atmosConfig, step.Prompt)
	if err != nil {
		return workflowStepFunctionError(step, "prompt", err)
	}
	step.Prompt = resolved

	resolved, err = resolveStepFunctionString(atmosConfig, step.Placeholder)
	if err != nil {
		return workflowStepFunctionError(step, "placeholder", err)
	}
	step.Placeholder = resolved

	for j := range step.Options {
		resolved, err = resolveStepFunctionString(atmosConfig, step.Options[j])
		if err != nil {
			return workflowStepFunctionError(step, "options", err)
		}
		step.Options[j] = resolved
	}
	return nil
}

// resolveStepFunctionString evaluates a single value-producing YAML function
// (!env or !exec) when the string is exactly such a function; otherwise it
// returns the input unchanged. Plain values and other (stack-dependent) tags
// pass through untouched.
func resolveStepFunctionString(atmosConfig *schema.AtmosConfiguration, value string) (string, error) {
	if !isWorkflowStepFunction(value) {
		return value, nil
	}
	// currentStack and stackInfo are intentionally empty/nil: only context-free
	// functions are gated in here, so they never need a stack context.
	result, err := processCustomTags(atmosConfig, value, "", nil, nil)
	if err != nil {
		return "", err
	}
	// Interactive step fields (default/prompt/options/placeholder) are scalars.
	// Reject a function that returns a non-string (e.g. !exec yielding JSON) so a
	// map/array is never silently stringified into a scalar field.
	s, ok := result.(string)
	if !ok {
		return "", fmt.Errorf("%w: function %q must return a string in a workflow step field, got %T",
			errUtils.ErrStepExecutionFailed, value, result)
	}
	return s, nil
}

// isWorkflowStepFunction reports whether value is a supported, context-free YAML
// function usable in workflow step fields.
func isWorkflowStepFunction(value string) bool {
	return matchesTag(value, u.AtmosYamlFuncEnv) || matchesTag(value, u.AtmosYamlFuncExec)
}

// workflowStepFunctionError wraps a YAML-function evaluation failure with the
// step name and field for a clear, actionable message.
func workflowStepFunctionError(step *schema.WorkflowStep, field string, cause error) error {
	return errUtils.Build(errUtils.ErrStepExecutionFailed).
		WithCause(cause).
		WithContext("step", step.Name).
		WithContext("field", field).
		WithExplanationf("Failed to evaluate the YAML function in step field %q", field).
		WithHint("Only !env and !exec are supported in workflow step fields").
		Err()
}
