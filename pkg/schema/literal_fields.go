package schema

import (
	"slices"
	"strings"
)

const (
	// LiteralFieldsKey is the internal key the loaders record on a step mapping to list the
	// fields written with the !literal tag. It travels in-band through merged configuration (like
	// script_source) and is never accepted from user input: the loaders strip a hand-written one.
	LiteralFieldsKey = "literal_fields"

	// LiteralFieldEnvPrefix prefixes the name of an individual env value in LiteralFields, for
	// example "env.GREETING".
	LiteralFieldEnvPrefix = "env."

	// LiteralFieldAmbientEnvPrefix identifies an already-rendered ambient environment value.
	// Unlike declared !literal markers, these runtime-only names retain exact casing so
	// separate Unix variables such as FOO and foo can have independent rendering policies.
	LiteralFieldAmbientEnvPrefix = "ambient_env."
)

// IsLiteral reports whether the named step field ("script", "command", "interpreter",
// "working_directory", "timeout") was written with the !literal tag and must be used exactly as written.
func (task *Task) IsLiteral(field string) bool {
	return slices.Contains(task.LiteralFields, field)
}

// IsLiteral reports whether the named step field ("script", "command", "interpreter",
// "working_directory", "timeout") was written with the !literal tag and must be used exactly as written.
func (step *WorkflowStep) IsLiteral(field string) bool {
	return slices.Contains(step.LiteralFields, field)
}

// IsLiteralEnv reports whether the env value with the given name was written with the !literal
// tag. See IsLiteralEnvName.
func (step *WorkflowStep) IsLiteralEnv(name string) bool {
	return IsLiteralEnvName(step.LiteralFields, name)
}

// SplitLiteralEnv separates env into the values to render as templates and the values written
// with the !literal tag, which must be used as written. Both results are non-nil.
func (step *WorkflowStep) SplitLiteralEnv(env map[string]string) (render, literal map[string]string) {
	render = make(map[string]string, len(env))
	literal = make(map[string]string)
	for key, value := range env {
		if step.IsLiteralEnv(key) {
			literal[key] = value
			continue
		}
		render[key] = value
	}
	return render, literal
}

// IsLiteralEnvName reports whether fields marks the env value with the given name as literal.
// Declared names match case-insensitively because configuration loading can change map-key case.
// Runtime ambient markers match exactly; they never pass through configuration normalization.
func IsLiteralEnvName(fields []string, name string) bool {
	for _, field := range fields {
		if envName, ok := strings.CutPrefix(field, LiteralFieldAmbientEnvPrefix); ok && envName == name {
			return true
		}
		if envName, ok := strings.CutPrefix(field, LiteralFieldEnvPrefix); ok && strings.EqualFold(envName, name) {
			return true
		}
	}
	return false
}

// LiteralFieldsFromValue decodes the in-band literal_fields value of a step payload (a list of
// strings, as a []string or a []any after a YAML round trip). Anything else yields nil.
func LiteralFieldsFromValue(value any) []string {
	switch list := value.(type) {
	case []string:
		return slices.Clone(list)
	case []any:
		fields := make([]string, 0, len(list))
		for _, item := range list {
			if field, ok := item.(string); ok {
				fields = append(fields, field)
			}
		}
		return fields
	default:
		return nil
	}
}

// LiteralStepFieldNames returns the step fields whose value can be marked with the !literal tag,
// besides the individual env values (see LiteralFieldEnvPrefix). Each of them is a scalar string
// field the step runner renders as a template unless it is marked literal.
func LiteralStepFieldNames() []string {
	return []string{"script", "command", "interpreter", "working_directory", "timeout"}
}

// StepKeyScript and StepKeyCommand are the keys that identify a mapping as a step when it appears
// in a `steps:` sequence, alongside the narrow container `run` form (see IsContainerRunStep).
const (
	StepKeyScript  = "script"
	StepKeyCommand = "command"
	StepKeyEnv     = "env"
	StepKeySteps   = "steps"
)

// IsContainerRunStep reports whether a step of the given type and action is a container `run`
// step. An empty action means `run`, the container default. Such a step keeps its command under
// `run:` or `with:` instead of a direct `command` key, yet its step-level fields (for example
// `working_directory`) can still be written with !literal, so the loaders recognize it as a step.
// Any other type, including other values of `action`, is not a container run step.
func IsContainerRunStep(stepType, action string) bool {
	if strings.TrimSpace(stepType) != containerStepType {
		return false
	}
	action = strings.TrimSpace(action)
	return action == "" || action == "run"
}
