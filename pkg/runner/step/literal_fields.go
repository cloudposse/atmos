package step

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ResolveStepField renders value as a template, unless the step marks field as literal (the
// step field was written with the !literal tag), in which case value is returned exactly as
// written. The field argument is a step field name such as "script", "command", "interpreter",
// or "working_directory".
func (v *Variables) ResolveStepField(step *schema.WorkflowStep, field, value string) (string, error) {
	defer perf.Track(nil, "step.Variables.ResolveStepField")()

	if step != nil && step.IsLiteral(field) {
		return value, nil
	}
	return v.Resolve(value)
}

// ResolveStepEnvMap renders the values of a step's env map as templates, except the values the
// step marks as literal (`env.NAME` written with the !literal tag), which are kept as written.
func (v *Variables) ResolveStepEnvMap(step *schema.WorkflowStep, envMap map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "step.Variables.ResolveStepEnvMap")()

	if step == nil || len(step.LiteralFields) == 0 {
		return v.ResolveEnvMap(envMap)
	}
	if envMap == nil {
		return nil, nil
	}
	result := make(map[string]string, len(envMap))
	for key, value := range envMap {
		if step.IsLiteralEnv(key) {
			result[key] = value
			continue
		}
		resolved, err := v.Resolve(value)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve env var %s: %w", key, err)
		}
		result[key] = resolved
	}
	return result, nil
}
