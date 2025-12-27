package workdir

import (
	"time"
)

// WorkdirMetadata stores metadata about a working directory.
// This is persisted to .workdir-metadata.json in each workdir.
type WorkdirMetadata struct {
	// Component is the component name.
	Component string `json:"component"`

	// Stack is the stack name (optional, for stack-specific workdirs).
	Stack string `json:"stack,omitempty"`

	// SourceType indicates the source type (always "local" for workdir provisioner).
	SourceType SourceType `json:"source_type"`

	// Source is the original component source path.
	Source string `json:"source,omitempty"`

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
)

// WorkdirConfig holds configuration for the workdir provisioner.
type WorkdirConfig struct {
	// Enabled controls whether workdir provisioning is active.
	// Defaults to false; set provision.workdir.enabled: true to enable.
	Enabled bool `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
}

// WorkdirPath returns the standard workdir directory name.
const WorkdirPath = ".workdir"

// WorkdirMetadataFile is the name of the metadata file in each workdir.
const WorkdirMetadataFile = ".workdir-metadata.json"

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
