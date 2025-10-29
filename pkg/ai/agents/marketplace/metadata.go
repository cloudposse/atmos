package marketplace

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
)

// AgentMetadata represents the .agent.yaml configuration file.
type AgentMetadata struct {
	// Basic information.
	Name        string `yaml:"name"`
	DisplayName string `yaml:"display_name"`
	Version     string `yaml:"version"`
	Author      string `yaml:"author"`
	Description string `yaml:"description"`
	Category    string `yaml:"category"`

	// Atmos compatibility.
	Atmos AtmosCompatibility `yaml:"atmos"`

	// Prompt configuration.
	Prompt PromptConfig `yaml:"prompt"`

	// Tool access (optional).
	Tools *ToolConfig `yaml:"tools,omitempty"`

	// Capabilities (optional).
	Capabilities []string `yaml:"capabilities,omitempty"`

	// Dependencies (optional).
	Dependencies []string `yaml:"dependencies,omitempty"`

	// Environment variables (optional).
	Env []EnvVar `yaml:"env,omitempty"`

	// Links.
	Repository    string `yaml:"repository"`
	Documentation string `yaml:"documentation,omitempty"`
}

// AtmosCompatibility defines Atmos version requirements.
type AtmosCompatibility struct {
	MinVersion string `yaml:"min_version"`
	MaxVersion string `yaml:"max_version,omitempty"` // Empty = no upper limit.
}

// PromptConfig specifies the prompt file location.
type PromptConfig struct {
	File string `yaml:"file"` // e.g., "prompt.md".
}

// ToolConfig specifies allowed and restricted tools.
type ToolConfig struct {
	Allowed    []string `yaml:"allowed,omitempty"`
	Restricted []string `yaml:"restricted,omitempty"`
}

// EnvVar represents a required or optional environment variable.
type EnvVar struct {
	Name        string `yaml:"name"`
	Required    bool   `yaml:"required"`
	Description string `yaml:"description,omitempty"`
}

// ParseAgentMetadata reads and parses an .agent.yaml file.
func ParseAgentMetadata(path string) (*AgentMetadata, error) {
	defer perf.Track(nil, "marketplace.ParseAgentMetadata")()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata file: %w", err)
	}

	var metadata AgentMetadata
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Basic validation.
	if err := validateMetadata(&metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

// validateMetadata performs basic validation on agent metadata.
func validateMetadata(m *AgentMetadata) error {
	if m.Name == "" {
		return &ValidationError{Field: "name", Message: "name is required"}
	}
	if m.DisplayName == "" {
		return &ValidationError{Field: "display_name", Message: "display_name is required"}
	}
	if m.Version == "" {
		return &ValidationError{Field: "version", Message: "version is required"}
	}
	if m.Author == "" {
		return &ValidationError{Field: "author", Message: "author is required"}
	}
	if m.Description == "" {
		return &ValidationError{Field: "description", Message: "description is required"}
	}
	if m.Category == "" {
		return &ValidationError{Field: "category", Message: "category is required"}
	}

	// Validate category.
	validCategories := map[string]bool{
		"general":      true,
		"analysis":     true,
		"refactor":     true,
		"security":     true,
		"validation":   true,
		"optimization": true,
	}
	if !validCategories[m.Category] {
		return &ValidationError{
			Field:   "category",
			Message: fmt.Sprintf("invalid category %q (must be one of: general, analysis, refactor, security, validation, optimization)", m.Category),
		}
	}

	// Validate Atmos compatibility.
	if m.Atmos.MinVersion == "" {
		return &ValidationError{Field: "atmos.min_version", Message: "atmos.min_version is required"}
	}

	// Validate prompt configuration.
	if m.Prompt.File == "" {
		return &ValidationError{Field: "prompt.file", Message: "prompt.file is required"}
	}

	// Validate repository URL.
	if m.Repository == "" {
		return &ValidationError{Field: "repository", Message: "repository is required"}
	}

	return nil
}
