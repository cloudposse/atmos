package hooks

import "strings"

// Hook is the structure for a hook configured in stack YAML.
// Each hook has a Kind that determines what engine runs it.
type Hook struct {
	// Kind selects the engine that runs this hook. Built-in kinds:
	// "store" (existing semantics), "command" (generic binary execution),
	// plus named tool kinds like "infracost", "checkov", "trivy", "kics".
	//
	// For back-compat, the legacy `command:` YAML key is accepted as an
	// alias when `kind:` is absent. See UnmarshalYAML.
	Kind string `yaml:"kind,omitempty"`

	Events []string `yaml:"events,omitempty"`

	// Generic command-kind fields. Used by the command engine and by named
	// tool kinds via their defaults.
	//
	// Command is the binary to execute (resolved via the toolchain). For
	// named kinds it defaults to the kind's binary; users may override.
	Command string            `yaml:"command,omitempty"`
	Args    []string          `yaml:"args,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	// Format is the inline rendering hint for generic kinds. v1 accepts
	// "markdown" or empty (= downloadable artifact, no inline render).
	Format string `yaml:"format,omitempty"`

	// OnFailure is the failure mode. "warn" (default for tool kinds),
	// "fail" (propagate non-zero exit), or "ignore" (swallow).
	OnFailure string `yaml:"on_failure,omitempty"`

	// Tfmigrate-kind specific fields.
	Migration     string   `yaml:"migration,omitempty"`
	Config        string   `yaml:"config,omitempty"`
	BackendConfig []string `yaml:"backend_config,omitempty"`
	Mode          string   `yaml:"mode,omitempty"`

	// Store-kind specific (existing, unchanged semantics).
	Name    string            `yaml:"name,omitempty"`
	Outputs map[string]string `yaml:"outputs,omitempty"`
}

// UnmarshalYAML decodes a Hook, accepting the legacy `command:` key as an
// alias for `kind:` when `kind:` is absent. Pre-existing stack manifests
// with `command: store` continue to parse identically post-rename.
func (h *Hook) UnmarshalYAML(unmarshal func(any) error) error {
	type hookAlias Hook // avoid recursion into UnmarshalYAML
	var aux hookAlias
	if err := unmarshal(&aux); err != nil {
		return err
	}
	*h = Hook(aux)

	// Legacy alias fix-up: if `kind:` is empty but `command:` is set, the
	// YAML is the pre-rename form where `command:` was the dispatch
	// discriminator (e.g. "store"). Promote it to Kind and clear Command.
	if h.Kind == "" && h.Command != "" {
		h.Kind = h.Command
		h.Command = ""
	}
	return nil
}

// MatchesEvent reports whether this hook should run for the given event.
// It normalises the yaml event format (hyphens, e.g. "after-terraform-apply")
// to the canonical Go format (dots, e.g. "after.terraform.apply") before
// comparing, so both styles are accepted in stack configuration.
//
// If the hook has no events configured, it matches all events to preserve
// backward compatibility with configs written before event filtering existed.
func (h Hook) MatchesEvent(event HookEvent) bool {
	if len(h.Events) == 0 {
		return true
	}
	normalizedEvent := event.Normalize()
	for _, e := range h.Events {
		if HookEvent(strings.ReplaceAll(e, "-", ".")).Normalize() == normalizedEvent {
			return true
		}
	}
	return false
}
