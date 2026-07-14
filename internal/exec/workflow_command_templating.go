package exec

import (
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// workflowCommandSupportsTemplating reports whether a workflow step type carries
// a raw command that is executed inline (outside the step-handler registry) and
// therefore must be resolved through the step variables before it runs. Handler
// step types such as input, choose, container, and toast resolve their own
// fields via the executor, so they are excluded here.
func workflowCommandSupportsTemplating(commandType string) bool {
	switch commandType {
	case "shell", "atmos", schema.TaskTypeExec:
		return true
	default:
		return false
	}
}

// resolveWorkflowStepCommand resolves step-variable templates
// ({{ .steps.* }}, {{ .env.* }}, {{ .flags.* }}, plus Sprig/Gomplate functions)
// in an inline workflow command using results captured by earlier steps. This
// gives workflow shell/atmos/exec steps the same access to prior step outputs
// that custom command steps already have.
//
// The step environment is overlaid as {{ .env.* }} for this call only (it does
// not mutate the shared executor's env), so per-step env does not leak across
// steps. It is a no-op when the shared step executor has not been initialized.
func resolveWorkflowStepCommand(command string, stepEnv []string) (string, error) {
	defer perf.Track(nil, "exec.resolveWorkflowStepCommand")()

	if command == "" || stepExecutorState == nil {
		return command, nil
	}
	return stepExecutorState.Variables().ResolveWith(command, envpkg.SliceToMap(stepEnv))
}

// resolveWorkflowStepEnvs resolves step-variable templates in both the
// workflow-level and step-level `env:` maps against the base environment, so the
// caller can perform a single error check before merging them.
func resolveWorkflowStepEnvs(workflowEnv, stepEnv map[string]string, baseEnv []string) (map[string]string, map[string]string, error) {
	defer perf.Track(nil, "exec.resolveWorkflowStepEnvs")()

	resolvedWorkflowEnv, err := resolveWorkflowStepEnv(workflowEnv, baseEnv)
	if err != nil {
		return nil, nil, err
	}
	resolvedStepEnv, err := resolveWorkflowStepEnv(stepEnv, baseEnv)
	if err != nil {
		return nil, nil, err
	}
	return resolvedWorkflowEnv, resolvedStepEnv, nil
}

// resolveWorkflowStepEnv resolves step-variable templates in the values of a
// workflow/step `env:` map, using prior step results and the base environment.
// This mirrors how custom command steps resolve their `env:` (ResolveEnvMap), so
// a value such as `COMPONENT: "{{ .steps.select.value }}"` is populated before
// it reaches the subprocess. Returns the input unchanged when there is nothing
// to resolve or the shared executor is not initialized.
func resolveWorkflowStepEnv(envMap map[string]string, baseEnv []string) (map[string]string, error) {
	defer perf.Track(nil, "exec.resolveWorkflowStepEnv")()

	if len(envMap) == 0 || stepExecutorState == nil {
		return envMap, nil
	}
	overlay := envpkg.SliceToMap(baseEnv)
	vars := stepExecutorState.Variables()
	resolved := make(map[string]string, len(envMap))
	for key, value := range envMap {
		rendered, err := vars.ResolveWith(value, overlay)
		if err != nil {
			return nil, err
		}
		resolved[key] = rendered
	}
	return resolved, nil
}
