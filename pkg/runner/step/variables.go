package step

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/template"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	defaultTemplatePasses = 1
	templateOpenDelim     = "{{"
	protectedTemplateOpen = "\x00ATMOS_TMPL_OPEN\x00"
)

// TemplateRenderer renders one template pass with the provided data.
type TemplateRenderer func(name, input string, data any) (string, error)

// Variables holds step outputs accessible via Go templates.
type Variables struct {
	// Steps maps step names to their results.
	Steps map[string]*StepResult
	// Env contains environment variables.
	Env map[string]string
	// Flags contains workflow command-line flags exposed to step templates.
	Flags            map[string]string
	AtmosConfig      *schema.AtmosConfiguration
	templateRoots    map[string]any
	templateRenderer TemplateRenderer
	templatePasses   int
	protectedRoots   map[string]struct{}
	// stageIndex tracks current stage position (1-indexed).
	stageIndex int
	// totalStages tracks total number of stage steps in workflow.
	totalStages int
}

// NewVariables creates a new Variables instance with OS environment pre-populated.
func NewVariables() *Variables {
	defer perf.Track(nil, "step.NewVariables")()

	v := &Variables{
		Steps:          make(map[string]*StepResult),
		Env:            make(map[string]string),
		Flags:          make(map[string]string),
		templateRoots:  make(map[string]any),
		templatePasses: defaultTemplatePasses,
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

// SetAtmosConfig stores the active Atmos configuration for step handlers that
// need to respect process-level settings such as native CI summary controls.
func (v *Variables) SetAtmosConfig(config *schema.AtmosConfiguration) {
	defer perf.Track(nil, "step.Variables.SetAtmosConfig")()

	v.AtmosConfig = config
}

// SetTemplateData sets extra root values exposed during template resolution.
func (v *Variables) SetTemplateData(data map[string]any) {
	defer perf.Track(nil, "step.Variables.SetTemplateData")()

	v.templateRoots = make(map[string]any, len(data))
	for key, value := range data {
		v.templateRoots[key] = value
	}
}

// SetTemplateRenderer sets the one-pass renderer used by Resolve.
func (v *Variables) SetTemplateRenderer(renderer TemplateRenderer) {
	defer perf.Track(nil, "step.Variables.SetTemplateRenderer")()

	v.templateRenderer = renderer
}

// SetTemplatePasses sets the maximum number of render passes used by Resolve.
func (v *Variables) SetTemplatePasses(passes int) {
	defer perf.Track(nil, "step.Variables.SetTemplatePasses")()

	if passes < 1 {
		passes = defaultTemplatePasses
	}
	v.templatePasses = passes
}

// ProtectTemplateRoots prevents template markers in selected roots from being
// re-evaluated during later render passes.
func (v *Variables) ProtectTemplateRoots(roots ...string) {
	defer perf.Track(nil, "step.Variables.ProtectTemplateRoots")()

	v.protectedRoots = make(map[string]struct{}, len(roots))
	for _, root := range roots {
		v.protectedRoots[root] = struct{}{}
	}
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
	data := map[string]any{
		"steps": steps,
		"env":   v.Env,
		"Flags": v.Flags,
		"flags": v.Flags,
	}
	for key, value := range v.templateRoots {
		data[key] = value
	}
	return data
}

// TemplateData returns the data structure used for Go template execution.
func (v *Variables) TemplateData() map[string]any {
	defer perf.Track(nil, "step.Variables.TemplateData")()

	return v.templateData()
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
		resolvedValue, err := v.resolveTemplate(fmt.Sprintf("step-output-%s", key), value, v.outputTemplateData(result))
		if err != nil {
			return nil, fmt.Errorf("failed to resolve output %s: %w", key, err)
		}
		resolved[key] = resolvedValue
	}
	return resolved, nil
}

// Resolve resolves Go templates in the given string using variable data.
func (v *Variables) Resolve(input string) (string, error) {
	defer perf.Track(nil, "step.Variables.Resolve")()

	if input == "" {
		return "", nil
	}

	return v.resolveTemplate("step", input, v.templateData())
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

func (v *Variables) resolveTemplate(name, input string, data map[string]any) (string, error) {
	safeData := v.protectTemplateData(data)
	passes := v.templatePasses
	if passes < 1 {
		passes = defaultTemplatePasses
	}

	result := input
	for pass := 0; pass < passes; pass++ {
		if pass > 0 && !strings.Contains(result, templateOpenDelim) {
			break
		}
		processed, err := v.renderTemplatePass(fmt.Sprintf("%s-pass-%d", name, pass+1), result, safeData)
		if err != nil {
			return "", err
		}
		result = processed
		if processed == input || processed == "" {
			break
		}
		input = processed
	}
	return strings.ReplaceAll(result, protectedTemplateOpen, templateOpenDelim), nil
}

func (v *Variables) renderTemplatePass(name, input string, data map[string]any) (string, error) {
	if v.templateRenderer != nil {
		return v.templateRenderer(name, input, data)
	}

	tmpl, err := template.New(name).Parse(input)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

func (v *Variables) protectTemplateData(data map[string]any) map[string]any {
	if len(v.protectedRoots) == 0 {
		return data
	}

	out := make(map[string]any, len(data))
	for key, value := range data {
		if _, ok := v.protectedRoots[key]; ok {
			out[key] = protectTemplateValue(value)
			continue
		}
		out[key] = value
	}
	return out
}

func protectTemplateValue(value any) any {
	switch typed := value.(type) {
	case string:
		return strings.ReplaceAll(typed, templateOpenDelim, protectedTemplateOpen)
	case map[string]string:
		out := make(map[string]string, len(typed))
		for key, value := range typed {
			out[key] = strings.ReplaceAll(value, templateOpenDelim, protectedTemplateOpen)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			if str, ok := value.(string); ok {
				out[key] = strings.ReplaceAll(str, templateOpenDelim, protectedTemplateOpen)
				continue
			}
			out[key] = value
		}
		return out
	case []string:
		out := make([]string, len(typed))
		for i, value := range typed {
			out[i] = strings.ReplaceAll(value, templateOpenDelim, protectedTemplateOpen)
		}
		return out
	default:
		return value
	}
}
