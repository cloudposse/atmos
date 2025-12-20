package step

import (
	"bytes"
	"fmt"
	"text/template"
)

// Variables holds step outputs accessible via Go templates.
type Variables struct {
	// Steps maps step names to their results.
	Steps map[string]*StepResult
	// Env contains environment variables.
	Env map[string]string
}

// NewVariables creates a new Variables instance.
func NewVariables() *Variables {
	return &Variables{
		Steps: make(map[string]*StepResult),
		Env:   make(map[string]string),
	}
}

// Set stores a step result by name.
func (v *Variables) Set(name string, result *StepResult) {
	v.Steps[name] = result
}

// GetValue returns a step's primary value.
func (v *Variables) GetValue(stepName string) (string, bool) {
	if result, ok := v.Steps[stepName]; ok {
		return result.Value, true
	}
	return "", false
}

// GetValues returns a step's multiple values.
func (v *Variables) GetValues(stepName string) ([]string, bool) {
	if result, ok := v.Steps[stepName]; ok {
		return result.Values, true
	}
	return nil, false
}

// SetEnv sets an environment variable.
func (v *Variables) SetEnv(key, value string) {
	v.Env[key] = value
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
