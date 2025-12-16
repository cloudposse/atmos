package workdir

import (
	"time"
)

// SourceConfig represents the metadata.source configuration for a component.
// It supports both a simple string form (just the URI) and a structured form
// with additional options like version and path filtering.
type SourceConfig struct {
	// URI is the go-getter compatible source URI.
	// Examples:
	//   - "github.com/cloudposse/terraform-aws-vpc?ref=v1.0.0"
	//   - "git::https://example.com/repo.git"
	//   - "s3::https://s3.amazonaws.com/bucket/path"
	URI string `yaml:"uri" json:"uri" mapstructure:"uri"`

	// Version is an optional version constraint or ref.
	// If specified, it's appended to the URI as a ref parameter.
	Version string `yaml:"version,omitempty" json:"version,omitempty" mapstructure:"version"`

	// IncludedPaths is a list of glob patterns for files to include.
	// If empty, all files are included.
	IncludedPaths []string `yaml:"included_paths,omitempty" json:"included_paths,omitempty" mapstructure:"included_paths"`

	// ExcludedPaths is a list of glob patterns for files to exclude.
	// Applied after included_paths filtering.
	ExcludedPaths []string `yaml:"excluded_paths,omitempty" json:"excluded_paths,omitempty" mapstructure:"excluded_paths"`
}

// WorkdirMetadata stores metadata about a working directory.
// This is persisted to .workdir-metadata.json in each workdir.
type WorkdirMetadata struct {
	// Component is the component name.
	Component string `json:"component"`

	// Stack is the stack name (optional, for stack-specific workdirs).
	Stack string `json:"stack,omitempty"`

	// SourceType indicates whether this is a local or remote source.
	SourceType SourceType `json:"source_type"`

	// SourceURI is the original source URI (for remote sources).
	SourceURI string `json:"source_uri,omitempty"`

	// SourceVersion is the version/ref used (for remote sources).
	SourceVersion string `json:"source_version,omitempty"`

	// LocalPath is the original local component path (for local sources).
	LocalPath string `json:"local_path,omitempty"`

	// CacheKey is the content-addressable cache key (for remote sources).
	CacheKey string `json:"cache_key,omitempty"`

	// CreatedAt is when the workdir was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the workdir was last updated.
	UpdatedAt time.Time `json:"updated_at"`

	// ContentHash is a hash of the source content for change detection.
	ContentHash string `json:"content_hash,omitempty"`
}

// SourceType indicates the type of component source.
type SourceType string

const (
	// SourceTypeLocal indicates the component source is a local path.
	SourceTypeLocal SourceType = "local"

	// SourceTypeRemote indicates the component source is a remote URI.
	SourceTypeRemote SourceType = "remote"
)

// CacheEntry represents an entry in the XDG source cache.
type CacheEntry struct {
	// Key is the content-addressable cache key (SHA256 of URI + version).
	Key string `json:"key"`

	// URI is the original source URI.
	URI string `json:"uri"`

	// Version is the version/ref used.
	Version string `json:"version,omitempty"`

	// Path is the filesystem path to the cached content.
	Path string `json:"path"`

	// CreatedAt is when the cache entry was created.
	CreatedAt time.Time `json:"created_at"`

	// LastAccessedAt is when the cache entry was last accessed.
	LastAccessedAt time.Time `json:"last_accessed_at"`

	// TTL is the time-to-live for this cache entry.
	// Zero means permanent (for tagged versions).
	TTL time.Duration `json:"ttl,omitempty"`

	// ContentHash is a hash of the cached content.
	ContentHash string `json:"content_hash"`
}

// CachePolicy determines how cache entries are managed.
type CachePolicy string

const (
	// CachePolicyPermanent means the cache entry never expires.
	// Used for tagged versions (e.g., v1.0.0) and commit SHAs.
	CachePolicyPermanent CachePolicy = "permanent"

	// CachePolicyTTL means the cache entry expires after a duration.
	// Used for branch refs (e.g., main, develop).
	CachePolicyTTL CachePolicy = "ttl"
)

// WorkdirConfig holds configuration for the workdir provisioner.
type WorkdirConfig struct {
	// Enabled controls whether workdir provisioning is active.
	// Defaults to false; set to true or use metadata.source to enable.
	Enabled bool `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`

	// CacheTTL is the time-to-live for branch-based cache entries.
	// Defaults to 1 hour.
	CacheTTL time.Duration `yaml:"cache_ttl,omitempty" json:"cache_ttl,omitempty" mapstructure:"cache_ttl"`
}

// DefaultCacheTTL is the default TTL for branch-based source cache entries.
const DefaultCacheTTL = 1 * time.Hour

// WorkdirPath returns the standard workdir directory name.
const WorkdirPath = ".workdir"

// WorkdirMetadataFile is the name of the metadata file in each workdir.
const WorkdirMetadataFile = ".workdir-metadata.json"

// CacheDir is the subdirectory under XDG cache for component sources.
const CacheDir = "components"

// File permission constants.
const (
	// DirPermissions is the default permission for created directories (rwxr-xr-x).
	DirPermissions = 0o755

	// FilePermissionsSecure is the secure permission for sensitive files (rw-------).
	FilePermissionsSecure = 0o600

	// FilePermissionsStandard is the standard permission for regular files (rw-r--r--).
	FilePermissionsStandard = 0o644
)

// String literal constants.
const (
	// ComponentKey is the key used to access component name in configuration.
	ComponentKey = "component"
)
