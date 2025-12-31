package hooks

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
