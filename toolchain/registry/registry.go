package registry

import (
	"context"
	"errors"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Error definitions for the registry package.
var (
	// ErrToolNotFound indicates a tool was not found in the registry.
	ErrToolNotFound = errors.New("tool not found")

	// ErrNoVersionsFound indicates no versions are available for a tool.
	ErrNoVersionsFound = errors.New("no versions found")

	// ErrInvalidToolSpec indicates the tool specification format is invalid.
	ErrInvalidToolSpec = errors.New("invalid tool specification")

	// ErrHTTPRequest indicates an HTTP request failed.
	ErrHTTPRequest = errors.New("HTTP request failed")

	// ErrHTTP404 indicates an HTTP 404 Not Found response.
	ErrHTTP404 = errors.New("HTTP 404 Not Found")

	// ErrRegistryParse indicates the registry file could not be parsed.
	ErrRegistryParse = errors.New("registry parse error")

	// ErrNoPackagesInRegistry indicates the registry contains no packages.
	ErrNoPackagesInRegistry = errors.New("no packages found in registry")

	// ErrNoAssetTemplate indicates no asset template is defined for the tool.
	ErrNoAssetTemplate = errors.New("no asset template defined")

	// ErrFileOperation indicates a file operation failed.
	ErrFileOperation = errors.New("file operation failed")

	// ErrUnknownRegistry indicates the registry name is not recognized.
	ErrUnknownRegistry = errors.New("unknown registry")

	// ErrRegistryNotRegistered indicates a registry factory has not been registered.
	ErrRegistryNotRegistered = errors.New("registry not registered")

	// ErrRegistryConfiguration indicates the registry configuration is invalid.
	ErrRegistryConfiguration = errors.New("invalid registry configuration")

	// ErrToolAlreadyExists indicates the tool version already exists in .tool-versions.
	ErrToolAlreadyExists = errors.New("tool already exists")
)

// SearchTotalProvider is an optional interface that registries can implement
// to provide the total count of search results (for pagination UI).
type SearchTotalProvider interface {
	GetLastSearchTotal() int
}

// ToolRegistry defines the interface for tool metadata registries.
// This abstraction allows multiple registry implementations (Aqua, custom URL-based, etc.)
// while keeping the toolchain package decoupled from specific registry formats.
type ToolRegistry interface {
	// GetTool fetches tool metadata from the registry.
	GetTool(owner, repo string) (*Tool, error)

	// GetToolWithVersion fetches tool metadata and resolves version-specific overrides.
	GetToolWithVersion(owner, repo, version string) (*Tool, error)

	// GetLatestVersion fetches the latest non-prerelease version for a tool.
	GetLatestVersion(owner, repo string) (string, error)

	// LoadLocalConfig is deprecated and will be removed. Returns nil for compatibility.
	LoadLocalConfig(configPath string) error

	// Search searches for tools matching the query string.
	// The query is matched against tool owner, repo, aliases, and description.
	// Results are sorted by relevance score.
	Search(ctx context.Context, query string, opts ...SearchOption) ([]*Tool, error)

	// ListAll returns all tools available in the registry.
	// Results can be paginated and sorted using options.
	ListAll(ctx context.Context, opts ...ListOption) ([]*Tool, error)

	// GetMetadata returns metadata about the registry itself.
	GetMetadata(ctx context.Context) (*RegistryMetadata, error)
}

// RegistryMetadata contains registry-level information.
type RegistryMetadata struct {
	Name        string
	Type        string // "aqua", "atmos"
	Source      string // URL
	Priority    int
	ToolCount   int
	LastUpdated time.Time
}

// SearchOption configures search behavior.
type SearchOption func(*SearchConfig)

// SearchConfig contains search configuration.
type SearchConfig struct {
	Limit         int
	Offset        int
	InstalledOnly bool
	AvailableOnly bool
}

// ListOption configures list behavior.
type ListOption func(*ListConfig)

// ListConfig contains list configuration.
type ListConfig struct {
	Limit  int
	Offset int
	Sort   string // "name", "date", "popularity"
}

// WithLimit sets the maximum number of results.
func WithLimit(limit int) SearchOption {
	defer perf.Track(nil, "registry.WithLimit")()

	return func(c *SearchConfig) {
		c.Limit = limit
	}
}

// WithOffset sets the starting offset for results.
func WithOffset(offset int) SearchOption {
	defer perf.Track(nil, "registry.WithOffset")()

	return func(c *SearchConfig) {
		c.Offset = offset
	}
}

// WithInstalledOnly filters to only show installed tools.
func WithInstalledOnly(installedOnly bool) SearchOption {
	defer perf.Track(nil, "registry.WithInstalledOnly")()

	return func(c *SearchConfig) {
		c.InstalledOnly = installedOnly
	}
}

// WithAvailableOnly filters to only show non-installed tools.
func WithAvailableOnly(availableOnly bool) SearchOption {
	defer perf.Track(nil, "registry.WithAvailableOnly")()

	return func(c *SearchConfig) {
		c.AvailableOnly = availableOnly
	}
}

// WithListLimit sets the maximum number of results for list operations.
func WithListLimit(limit int) ListOption {
	defer perf.Track(nil, "registry.WithListLimit")()

	return func(c *ListConfig) {
		c.Limit = limit
	}
}

// WithListOffset sets the starting offset for list operations.
func WithListOffset(offset int) ListOption {
	defer perf.Track(nil, "registry.WithListOffset")()

	return func(c *ListConfig) {
		c.Offset = offset
	}
}

// WithSort sets the sort order for list operations.
func WithSort(sort string) ListOption {
	defer perf.Track(nil, "registry.WithSort")()

	return func(c *ListConfig) {
		c.Sort = sort
	}
}

// Tool represents a single tool in the registry.
type Tool struct {
	Name             string            `yaml:"name"`
	Registry         string            `yaml:"registry"`
	Version          string            `yaml:"version"`
	Type             string            `yaml:"type"`
	RepoOwner        string            `yaml:"repo_owner"`
	RepoName         string            `yaml:"repo_name"`
	Asset            string            `yaml:"asset"`
	URL              string            `yaml:"url"`
	Format           string            `yaml:"format"`
	Files            []File            `yaml:"files"`
	Overrides        []Override        `yaml:"overrides"`
	VersionOverrides []VersionOverride `yaml:"version_overrides"`
	SupportedIf      *SupportedIf      `yaml:"supported_if"`
	Replacements     map[string]string `yaml:"replacements"`
	BinaryName       string            `yaml:"binary_name"`
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
	Name       string `yaml:"name"` // Used by http and some go_install types.
	Path       string `yaml:"path"` // Used by go_install types (Go module path).
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
	VersionConstraint   string            `yaml:"version_constraint"`
	Asset               string            `yaml:"asset"`
	Format              string            `yaml:"format"`
	Rosetta2            bool              `yaml:"rosetta2"`
	WindowsArmEmulation bool              `yaml:"windows_arm_emulation"`
	SupportedEnvs       []string          `yaml:"supported_envs"`
	Checksum            ChecksumConfig    `yaml:"checksum"`
	Files               []File            `yaml:"files"`
	Replacements        map[string]string `yaml:"replacements"`
}

// AquaRegistryFile represents the structure of an Aqua registry YAML file (uses 'packages' key).
type AquaRegistryFile struct {
	Packages []AquaPackage `yaml:"packages"`
}
