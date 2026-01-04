package toolmgr

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/tools/director/internal/scene"
)

// DefaultsConfig represents the full defaults.yaml structure.
type DefaultsConfig struct {
	Version    string                  `yaml:"version"`
	Hooks      *HooksConfig            `yaml:"hooks,omitempty"`
	Tools      *ToolsConfig            `yaml:"tools,omitempty"`
	Validation *scene.ValidationConfig `yaml:"validation,omitempty"` // Default validation rules for rendered outputs.
}

// HooksConfig contains hooks to run at various stages.
type HooksConfig struct {
	PreRender []string `yaml:"pre_render,omitempty"` // Commands to run before VHS rendering.
}

// LoadToolsConfig loads the tools configuration from defaults.yaml.
func LoadToolsConfig(demosDir string) (*ToolsConfig, error) {
	defaultsFile := demosDir + "/defaults.yaml"

	data, err := os.ReadFile(defaultsFile)
	if os.IsNotExist(err) {
		return nil, nil // No defaults.yaml, no tools config.
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read defaults.yaml: %w", err)
	}

	var config DefaultsConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse defaults.yaml: %w", err)
	}

	return config.Tools, nil
}

// LoadDefaultsConfig loads the full defaults configuration from defaults.yaml.
func LoadDefaultsConfig(demosDir string) (*DefaultsConfig, error) {
	defaultsFile := demosDir + "/defaults.yaml"

	data, err := os.ReadFile(defaultsFile)
	if os.IsNotExist(err) {
		return nil, nil // No defaults.yaml.
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read defaults.yaml: %w", err)
	}

	var config DefaultsConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse defaults.yaml: %w", err)
	}

	return &config, nil
}
