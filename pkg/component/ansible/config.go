package ansible

import (
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Config represents the configuration structure for ansible components.
// This configuration mirrors the schema.Ansible struct from pkg/schema/schema.go.
// and is used for type-safe configuration access within the provider.
type Config struct {
	// BasePath is the filesystem path to ansible components.
	BasePath string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`

	// Command is the ansible binary to use (default: ansible).
	Command string `yaml:"command" json:"command" mapstructure:"command"`

	// AutoGenerateFiles enables automatic generation of auxiliary configuration files
	// during Ansible operations when set to true.
	// Generated files are defined in the component's generate section.
	AutoGenerateFiles bool `yaml:"auto_generate_files" json:"auto_generate_files" mapstructure:"auto_generate_files"`
}

// DefaultConfig returns the default configuration for ansible components.
func DefaultConfig() Config {
	defer perf.Track(nil, "ansible.DefaultConfig")()

	return Config{
		BasePath:          filepath.Join("components", "ansible"),
		Command:           "ansible",
		AutoGenerateFiles: false,
	}
}
