package registry

import (
	"errors"
	"fmt"
	"os"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Error definitions for the registry package.
var (
	// ErrToolNotFound indicates a tool was not found in the registry or local configuration.
	ErrToolNotFound = errors.New("tool not found")

	// ErrNoVersionsFound indicates no versions are available for a tool.
	ErrNoVersionsFound = errors.New("no versions found")

	// ErrInvalidToolSpec indicates the tool specification format is invalid.
	ErrInvalidToolSpec = errors.New("invalid tool specification")

	// ErrHTTPRequest indicates an HTTP request failed.
	ErrHTTPRequest = errors.New("HTTP request failed")

	// ErrRegistryParse indicates the registry file could not be parsed.
	ErrRegistryParse = errors.New("registry parse error")

	// ErrNoPackagesInRegistry indicates the registry contains no packages.
	ErrNoPackagesInRegistry = errors.New("no packages found in registry")

	// ErrNoAssetTemplate indicates no asset template is defined for the tool.
	ErrNoAssetTemplate = errors.New("no asset template defined")

	// ErrFileOperation indicates a file operation failed.
	ErrFileOperation = errors.New("file operation failed")
)

// ToolRegistry defines the interface for tool metadata registries.
// This abstraction allows multiple registry implementations (Aqua, custom, local-only, etc.)
// while keeping the toolchain package decoupled from specific registry formats.
type ToolRegistry interface {
	// GetTool fetches tool metadata from the registry.
	GetTool(owner, repo string) (*Tool, error)

	// GetToolWithVersion fetches tool metadata and resolves version-specific overrides.
	GetToolWithVersion(owner, repo, version string) (*Tool, error)

	// GetLatestVersion fetches the latest non-prerelease version for a tool.
	GetLatestVersion(owner, repo string) (string, error)

	// LoadLocalConfig loads local configuration overrides.
	LoadLocalConfig(configPath string) error
}

// Tool represents a single tool in the registry.
type Tool struct {
	Name         string            `yaml:"name"`
	Registry     string            `yaml:"registry"`
	Version      string            `yaml:"version"`
	Type         string            `yaml:"type"`
	RepoOwner    string            `yaml:"repo_owner"`
	RepoName     string            `yaml:"repo_name"`
	Asset        string            `yaml:"asset"`
	URL          string            `yaml:"url"`
	Format       string            `yaml:"format"`
	Files        []File            `yaml:"files"`
	Overrides    []Override        `yaml:"overrides"`
	SupportedIf  *SupportedIf      `yaml:"supported_if"`
	Replacements map[string]string `yaml:"replacements"`
	BinaryName   string            `yaml:"binary_name"`
}

// File represents a file to be extracted from the archive.
type File struct {
	Name string `yaml:"name"`
	Src  string `yaml:"src"`
}

// Override represents platform-specific overrides.
type Override struct {
	GOOS   string `yaml:"goos"`
	GOARCH string `yaml:"goarch"`
	Asset  string `yaml:"asset"`
	Files  []File `yaml:"files"`
}

// SupportedIf represents conditions for when a tool is supported.
type SupportedIf struct {
	GOOS   string `yaml:"goos"`
	GOARCH string `yaml:"goarch"`
}

// ToolRegistryFile represents the structure of a tool registry YAML file.
type ToolRegistryFile struct {
	Tools []Tool `yaml:"tools"`
}

// AquaPackage represents a single package in the Aqua registry format.
// This struct matches the Aqua registry YAML fields exactly
// and is used only for parsing Aqua registry files.
type AquaPackage struct {
	Type       string `yaml:"type"`
	RepoOwner  string `yaml:"repo_owner"`
	RepoName   string `yaml:"repo_name"`
	URL        string `yaml:"url"`
	Format     string `yaml:"format"`
	BinaryName string `yaml:"binary_name"`
	// Add other Aqua fields as needed
	Description       string            `yaml:"description"`
	SupportedEnvs     []string          `yaml:"supported_envs"`
	Checksum          ChecksumConfig    `yaml:"checksum"`
	VersionConstraint string            `yaml:"version_constraint"`
	VersionOverrides  []VersionOverride `yaml:"version_overrides"`
}

// ChecksumConfig represents checksum configuration for Aqua packages.
type ChecksumConfig struct {
	Type      string `yaml:"type"`
	URL       string `yaml:"url"`
	Algorithm string `yaml:"algorithm"`
}

// VersionOverride represents version-specific overrides for Aqua packages.
type VersionOverride struct {
	VersionConstraint string `yaml:"version_constraint"`
	Rosetta2          bool   `yaml:"rosetta2"`
}

// AquaRegistryFile represents the structure of an Aqua registry YAML file (uses 'packages' key).
type AquaRegistryFile struct {
	Packages []AquaPackage `yaml:"packages"`
}

// LocalConfig represents the local tools.yaml configuration.
type LocalConfig struct {
	Aliases map[string]string    `yaml:"aliases"`
	Tools   map[string]LocalTool `yaml:"tools"`
}

// LocalTool represents a tool definition in the local config.
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

// LocalVersionConstraint represents a version constraint in local config.
type LocalVersionConstraint struct {
	Constraint string `yaml:"constraint"`
	Asset      string `yaml:"asset"`
	Format     string `yaml:"format"`
	BinaryName string `yaml:"binary_name"`
}

// LocalConfigManager handles local configuration.
type LocalConfigManager struct {
	config *LocalConfig
}

// NewLocalConfigManager creates a new local config manager.
func NewLocalConfigManager() *LocalConfigManager {
	defer perf.Track(nil, "registry.NewLocalConfigManager")()

	return &LocalConfigManager{}
}

// NewLocalConfigManagerWithConfig creates a new local config manager with the given config (for testing).
func NewLocalConfigManagerWithConfig(config *LocalConfig) *LocalConfigManager {
	defer perf.Track(nil, "registry.NewLocalConfigManagerWithConfig")()

	return &LocalConfigManager{config: config}
}

// Load loads the local tools.yaml configuration.
func (lcm *LocalConfigManager) Load(configPath string) error {
	defer perf.Track(nil, "registry.LocalConfigManager.Load")()

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

// GetTool returns a local tool definition if it exists.
func (lcm *LocalConfigManager) GetTool(owner, repo string) (*LocalTool, bool) {
	defer perf.Track(nil, "registry.LocalConfigManager.GetTool")()

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

// ResolveAlias resolves a tool name to its owner/repo path using aliases.
func (lcm *LocalConfigManager) ResolveAlias(toolName string) (string, bool) {
	defer perf.Track(nil, "registry.LocalConfigManager.ResolveAlias")()

	if lcm.config == nil || lcm.config.Aliases == nil {
		return "", false
	}

	alias, exists := lcm.config.Aliases[toolName]
	if !exists {
		return "", false
	}

	return alias, true
}

// GetToolConfig returns a tool configuration by owner/repo path.
func (lcm *LocalConfigManager) GetToolConfig(ownerRepo string) (*LocalTool, bool) {
	defer perf.Track(nil, "registry.LocalConfigManager.GetToolConfig")()

	if lcm.config == nil {
		return nil, false
	}

	tool, exists := lcm.config.Tools[ownerRepo]
	if !exists {
		return nil, false
	}

	return &tool, true
}

// GetAliases returns the aliases map from the config.
func (lcm *LocalConfigManager) GetAliases() map[string]string {
	defer perf.Track(nil, "registry.LocalConfigManager.GetAliases")()

	if lcm.config == nil {
		return nil
	}

	return lcm.config.Aliases
}

// ResolveVersionConstraint resolves the appropriate version constraint for a given version.
func (lcm *LocalConfigManager) ResolveVersionConstraint(tool *LocalTool, version string) *LocalVersionConstraint {
	defer perf.Track(nil, "registry.LocalConfigManager.ResolveVersionConstraint")()

	if len(tool.VersionConstraints) == 0 {
		return nil
	}

	v, err := semver.NewVersion(version)
	if err != nil {
		return nil // Invalid version string
	}

	for _, constraint := range tool.VersionConstraints {
		c, err := semver.NewConstraint(constraint.Constraint)
		if err != nil {
			continue // Skip invalid constraints
		}
		if c.Check(v) {
			return &constraint
		}
	}

	// Return the last constraint as fallback
	if len(tool.VersionConstraints) > 0 {
		return &tool.VersionConstraints[len(tool.VersionConstraints)-1]
	}

	return nil
}

// GetToolWithVersion returns a Tool for the given owner/repo and version, using local config (asdf-style versioned local tool lookup).
func (lcm *LocalConfigManager) GetToolWithVersion(owner, repo, version string) (*Tool, error) {
	defer perf.Track(nil, "registry.LocalConfigManager.GetToolWithVersion")()

	tool, exists := lcm.GetTool(owner, repo)
	if !exists {
		return nil, fmt.Errorf("%w: tool %s/%s not found in local config", ErrToolNotFound, owner, repo)
	}

	constraint := lcm.ResolveVersionConstraint(tool, version)

	t := &Tool{
		Name:       repo,
		Type:       tool.Type,
		RepoOwner:  owner,
		RepoName:   repo,
		Asset:      tool.Asset,
		URL:        tool.URL,
		Format:     tool.Format,
		BinaryName: tool.BinaryName,
		Version:    version,
	}

	// Set binary name if specified
	if tool.BinaryName != "" {
		t.Name = tool.BinaryName
	}

	// Handle URL for http type (copy URL to Asset for compatibility)
	if tool.Type == "http" {
		t.Asset = tool.URL
	}

	if constraint != nil {
		if constraint.Asset != "" {
			t.Asset = constraint.Asset
		}
		if constraint.Format != "" {
			t.Format = constraint.Format
		}
		if constraint.BinaryName != "" {
			t.Name = constraint.BinaryName
		}
	}

	return t, nil
}

// DefaultRegistry returns the default registry implementation (Aqua).
// This is a convenience function to avoid importing toolchain/registry/aqua directly
// in most use cases.
func DefaultRegistry() ToolRegistry {
	defer perf.Track(nil, "registry.DefaultRegistry")()

	// We import aqua dynamically here to avoid circular imports
	// while still providing a convenient default
	return nil // Will be implemented after moving aqua
}
