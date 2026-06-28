package step

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/template"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Variables holds step outputs accessible via Go templates.
type Variables struct {
	// Steps maps step names to their results.
	Steps map[string]*StepResult
	// Env contains environment variables.
	Env map[string]string
	// Flags contains workflow command-line flags exposed to step templates.
	Flags map[string]string
	// stageIndex tracks current stage position (1-indexed).
	stageIndex int
	// totalStages tracks total number of stage steps in workflow.
	totalStages int
}

// NewVariables creates a new Variables instance with OS environment pre-populated.
func NewVariables() *Variables {
	defer perf.Track(nil, "step.NewVariables")()

	v := &Variables{
		Steps: make(map[string]*StepResult),
		Env:   make(map[string]string),
		Flags: make(map[string]string),
	}
	// Pre-populate with OS environment variables.
	v.LoadOSEnv()
	return v
}

// LoadOSEnv populates the Env map with all OS environment variables.
func (v *Variables) LoadOSEnv() {
	defer perf.Track(nil, "step.Variables.LoadOSEnv")()

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			v.Env[parts[0]] = parts[1]
		}
	}
}

// Set stores a step result by name.
func (v *Variables) Set(name string, result *StepResult) {
	defer perf.Track(nil, "step.Variables.Set")()

	v.Steps[name] = result
}

// SetWithOutputs evaluates declared outputs and stores a step result by name.
func (v *Variables) SetWithOutputs(name string, result *StepResult, outputs map[string]string) error {
	defer perf.Track(nil, "step.Variables.SetWithOutputs")()

	if result == nil {
		return nil
	}
	if len(outputs) > 0 {
		evaluated, err := v.ResolveOutputs(outputs, result)
		if err != nil {
			return err
		}
		if result.Outputs == nil {
			result.Outputs = make(map[string]string)
		}
		for k, val := range evaluated {
			result.Outputs[k] = val
		}
	}
	v.Set(name, result)
	return nil
}

// EnvSlice returns the variable environment as a sorted slice of "KEY=VALUE"
// entries, suitable for use as a subprocess environment. The slice is the
// complete environment (it includes the inherited OS environment loaded by
// NewVariables plus any workflow/step/identity-resolved entries), so callers can
// hand it to a container runtime via container.EnvSetter to carry credentials
// materialized by auth integrations (e.g. DOCKER_CONFIG for ECR login).
func (v *Variables) EnvSlice() []string {
	defer perf.Track(nil, "step.Variables.EnvSlice")()

	keys := make([]string, 0, len(v.Env))
	for k := range v.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	env := make([]string, 0, len(keys))
	for _, k := range keys {
		env = append(env, k+"="+v.Env[k])
	}
	return env
}

// GetValue returns a step's primary value.
func (v *Variables) GetValue(stepName string) (string, bool) {
	defer perf.Track(nil, "step.Variables.GetValue")()

	if result, ok := v.Steps[stepName]; ok {
		return result.Value, true
	}
	return "", false
}

// GetValues returns a step's multiple values.
func (v *Variables) GetValues(stepName string) ([]string, bool) {
	defer perf.Track(nil, "step.Variables.GetValues")()

	if result, ok := v.Steps[stepName]; ok {
		return result.Values, true
	}
	return nil, false
}

// SetEnv sets an environment variable.
func (v *Variables) SetEnv(key, value string) {
	defer perf.Track(nil, "step.Variables.SetEnv")()

	v.Env[key] = value
}

// SetFlag sets a workflow flag variable.
func (v *Variables) SetFlag(key, value string) {
	defer perf.Track(nil, "step.Variables.SetFlag")()

	if v.Flags == nil {
		v.Flags = make(map[string]string)
	}
	v.Flags[key] = value
}

// SetTotalStages sets the total number of stage steps in the workflow.
func (v *Variables) SetTotalStages(total int) {
	defer perf.Track(nil, "step.Variables.SetTotalStages")()

	v.totalStages = total
}

// GetTotalStages returns the total number of stage steps.
func (v *Variables) GetTotalStages() int {
	defer perf.Track(nil, "step.Variables.GetTotalStages")()

	return v.totalStages
}

// IncrementStageIndex increments and returns the current stage index.
func (v *Variables) IncrementStageIndex() int {
	defer perf.Track(nil, "step.Variables.IncrementStageIndex")()

	v.stageIndex++
	return v.stageIndex
}

// GetStageIndex returns the current stage index.
func (v *Variables) GetStageIndex() int {
	defer perf.Track(nil, "step.Variables.GetStageIndex")()

	return v.stageIndex
}

// templateData returns the data structure for Go template execution.
func (v *Variables) templateData() map[string]any {
	steps := make(map[string]any, len(v.Steps))
	for name, result := range v.Steps {
		stepData := map[string]any{
			"value":    result.Value,
			"values":   result.Values,
			"metadata": result.Metadata,
			"outputs":  result.Outputs,
			"skipped":  result.Skipped,
			"error":    result.Error,
		}
		for key, value := range result.Metadata {
			stepData[key] = value
		}
		steps[name] = stepData
	}
	return map[string]any{
		"steps": steps,
		"env":   v.Env,
		"Flags": v.Flags,
		"flags": v.Flags,
	}
}

func (v *Variables) outputTemplateData(result *StepResult) map[string]any {
	data := v.templateData()
	data["value"] = result.Value
	data["values"] = result.Values
	data["metadata"] = result.Metadata
	data["outputs"] = result.Outputs
	data["skipped"] = result.Skipped
	data["error"] = result.Error
	for key, value := range result.Metadata {
		data[key] = value
	}
	return data
}

// ResolveOutputs resolves a step's declared outputs against the current result
// and all previously stored step results.
func (v *Variables) ResolveOutputs(outputs map[string]string, result *StepResult) (map[string]string, error) {
	defer perf.Track(nil, "step.Variables.ResolveOutputs")()

	resolved := make(map[string]string, len(outputs))
	for key, value := range outputs {
		tmpl, err := template.New("step-output").Parse(value)
		if err != nil {
			return nil, fmt.Errorf("failed to parse output %s: %w", key, err)
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, v.outputTemplateData(result)); err != nil {
			return nil, fmt.Errorf("failed to execute output %s: %w", key, err)
		}
		resolved[key] = buf.String()
	}
	return resolved, nil
}

// Resolve resolves Go templates in the given string using variable data.
func (v *Variables) Resolve(input string) (string, error) {
	defer perf.Track(nil, "step.Variables.Resolve")()

	if input == "" {
		return "", nil
	}

	tmpl, err := template.New("step").Parse(input)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, v.templateData()); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// ResolveEnvMap resolves Go templates in a map of environment variables.
func (v *Variables) ResolveEnvMap(envMap map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "step.Variables.ResolveEnvMap")()

	if envMap == nil {
		return nil, nil
	}

	result := make(map[string]string, len(envMap))
	for key, value := range envMap {
		resolved, err := v.Resolve(value)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve env var %s: %w", key, err)
		}
		result[key] = resolved
	}
	return result, nil
}
