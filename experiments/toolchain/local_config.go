package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// LocalConfig represents the local tools.yaml configuration
type LocalConfig struct {
	Aliases map[string]string    `yaml:"aliases"`
	Tools   map[string]LocalTool `yaml:"tools"`
}

// LocalTool represents a tool definition in the local config
type LocalTool struct {
	Type               string                   `yaml:"type"`
	URL                string                   `yaml:"url"`
	RepoOwner          string                   `yaml:"repo_owner"`
	RepoName           string                   `yaml:"repo_name"`
	Asset              string                   `yaml:"asset"`
	Format             string                   `yaml:"format"`
	BinaryName         string                   `yaml:"binary_name"`
	VersionConstraints []LocalVersionConstraint `yaml:"version_constraints"`
}

// LocalVersionConstraint represents a version constraint in local config
type LocalVersionConstraint struct {
	Constraint string `yaml:"constraint"`
	Asset      string `yaml:"asset"`
	Format     string `yaml:"format"`
	BinaryName string `yaml:"binary_name"`
}

// LocalConfigManager handles local configuration
type LocalConfigManager struct {
	config *LocalConfig
}

// NewLocalConfigManager creates a new local config manager
func NewLocalConfigManager() *LocalConfigManager {
	return &LocalConfigManager{}
}

// Load loads the local tools.yaml configuration
func (lcm *LocalConfigManager) Load(configPath string) error {
	if configPath == "" {
		configPath = "tools.yaml"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No local config file, that's okay
			lcm.config = &LocalConfig{Tools: make(map[string]LocalTool)}
			return nil
		}
		return fmt.Errorf("failed to read local config: %w", err)
	}

	var config LocalConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse local config: %w", err)
	}

	lcm.config = &config
	return nil
}

// GetTool returns a local tool definition if it exists
func (lcm *LocalConfigManager) GetTool(owner, repo string) (*LocalTool, bool) {
	if lcm.config == nil {
		return nil, false
	}

	key := fmt.Sprintf("%s/%s", owner, repo)
	tool, exists := lcm.config.Tools[key]
	if !exists {
		return nil, false
	}

	return &tool, true
}

// ResolveAlias resolves a tool name to its owner/repo path using aliases
func (lcm *LocalConfigManager) ResolveAlias(toolName string) (string, bool) {
	if lcm.config == nil || lcm.config.Aliases == nil {
		return "", false
	}

	alias, exists := lcm.config.Aliases[toolName]
	if !exists {
		return "", false
	}

	return alias, true
}

// GetToolConfig returns a tool configuration by owner/repo path
func (lcm *LocalConfigManager) GetToolConfig(ownerRepo string) (*LocalTool, bool) {
	if lcm.config == nil {
		return nil, false
	}

	tool, exists := lcm.config.Tools[ownerRepo]
	if !exists {
		return nil, false
	}

	return &tool, true
}

// ResolveVersionConstraint resolves the appropriate version constraint for a given version
func (lcm *LocalConfigManager) ResolveVersionConstraint(tool *LocalTool, version string) *LocalVersionConstraint {
	if len(tool.VersionConstraints) == 0 {
		return nil
	}

	// For now, use the first matching constraint
	// TODO: Implement proper semver constraint evaluation
	for _, constraint := range tool.VersionConstraints {
		// Simple string matching for now
		if strings.Contains(constraint.Constraint, ">=") {
			// Extract version from constraint like ">= 1.10.0"
			constraintVersion := strings.TrimSpace(strings.TrimPrefix(constraint.Constraint, ">="))
			if version >= constraintVersion {
				return &constraint
			}
		} else if strings.Contains(constraint.Constraint, "<") {
			// Extract version from constraint like "< 1.10.0"
			constraintVersion := strings.TrimSpace(strings.TrimPrefix(constraint.Constraint, "<"))
			if version < constraintVersion {
				return &constraint
			}
		}
	}

	// Return the last constraint as fallback
	if len(tool.VersionConstraints) > 0 {
		return &tool.VersionConstraints[len(tool.VersionConstraints)-1]
	}

	return nil
}
