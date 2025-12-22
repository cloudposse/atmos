package toolmgr

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// DefaultsConfig represents the full defaults.yaml structure.
type DefaultsConfig struct {
	Version string       `yaml:"version"`
	Tools   *ToolsConfig `yaml:"tools,omitempty"`
	// Other fields not relevant to toolmgr.
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
