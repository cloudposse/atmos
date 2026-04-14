package hooks

import "strings"

// Hook is the structure for a hook and is using in the stack config to define
// a command that should be run when a specific event occurs.
type Hook struct {
	Events  []string `yaml:"events"`
	Command string   `yaml:"command"`

	// Dynamic command-specific properties.

	// store command.
	Name    string            `yaml:"name,omitempty"`    // for store command
	Outputs map[string]string `yaml:"outputs,omitempty"` // for store command

	// Note: CI commands (ci.upload, ci.download, ci.summary) are deprecated.
	// Use RunCIHooks which automatically triggers CI actions based on
	// component provider bindings. See pkg/ci/ for the modern implementation.
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
