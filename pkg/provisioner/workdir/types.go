package workdir

import (
	"path/filepath"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// WorkdirMetadata stores metadata about a working directory.
// This is persisted to .atmos/metadata.json in each workdir.
type WorkdirMetadata struct {
	// Component is the component name.
	Component string `json:"component"`

	// Stack is the stack name (optional, for stack-specific workdirs).
	Stack string `json:"stack,omitempty"`

	// SourceType indicates the source type ("local" or "remote").
	SourceType SourceType `json:"source_type"`

	// Source is the original component source path.
	Source string `json:"source,omitempty"`

	// SourceURI is the remote source URI (for remote sources).
	SourceURI string `json:"source_uri,omitempty"`

	// SourceVersion is the remote source version (for remote sources).
	SourceVersion string `json:"source_version,omitempty"`

	// CreatedAt is when the workdir was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the workdir was last updated.
	UpdatedAt time.Time `json:"updated_at"`

	// LastAccessed is when the workdir was last accessed (for TTL tracking).
	LastAccessed time.Time `json:"last_accessed,omitempty"`

	// ContentHash is a hash of the source content for change detection (local sources only).
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

// WorkdirConfig holds configuration for the workdir provisioner.
type WorkdirConfig struct {
	// Enabled controls whether workdir provisioning is active.
	// Defaults to false; set provision.workdir.enabled: true to enable.
	Enabled bool `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
}

// WorkdirPath returns the standard workdir directory name.
const WorkdirPath = ".workdir"

// AtmosDir is the Atmos-specific directory within each workdir.
const AtmosDir = ".atmos"

// MetadataFile is the name of the metadata file within AtmosDir.
const MetadataFile = "metadata.json"

// WorkdirMetadataFile is the legacy name of the metadata file.
// Deprecated: Use MetadataPath() instead.
const WorkdirMetadataFile = ".workdir-metadata.json"

// MetadataPath returns the full path to the metadata file within a workdir.
func MetadataPath(workdirPath string) string {
	defer perf.Track(nil, "workdir.MetadataPath")()

	return filepath.Join(workdirPath, AtmosDir, MetadataFile)
}

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

	// WorkdirPathKey is the key used to store/retrieve the workdir path in component configuration.
	// This is set by the workdir provisioner and checked by terraform execution to override
	// the component path with the workdir path.
	WorkdirPathKey = "_workdir_path"
)
