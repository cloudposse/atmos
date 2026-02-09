package step

import (
	"bytes"
	"fmt"
	"os"
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
		steps[name] = map[string]any{
			"value":    result.Value,
			"values":   result.Values,
			"metadata": result.Metadata,
			"skipped":  result.Skipped,
			"error":    result.Error,
		}
	}
	return map[string]any{
		"steps": steps,
		"env":   v.Env,
	}
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
