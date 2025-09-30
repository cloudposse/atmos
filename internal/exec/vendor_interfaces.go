package exec

import (
	"os"

	"github.com/cloudposse/atmos/pkg/schema"
)

// GitVersionChecker is an interface for checking Git repository versions.
//
//go:generate mockgen -source=vendor_interfaces.go -destination=mock_vendor_interfaces.go -package=exec GitVersionChecker
type GitVersionChecker interface {
	// GetLatestTag returns the latest semantic version tag from a Git repository.
	GetLatestTag(repoURL string) (string, error)
	// GetLatestCommit returns the latest commit hash from a Git repository.
	GetLatestCommit(repoURL string) (string, error)
	// IsVersionNewer checks if newVersion is newer than currentVersion.
	IsVersionNewer(currentVersion, newVersion string) bool
}

// FileUpdater is an interface for updating files.
//
//go:generate mockgen -source=vendor_interfaces.go -destination=mock_vendor_interfaces.go -package=exec FileUpdater
type FileUpdater interface {
	// ReadFile reads a file from the filesystem.
	ReadFile(path string) ([]byte, error)
	// WriteFile writes data to a file on the filesystem.
	WriteFile(path string, data []byte, perm os.FileMode) error
	// FileExists checks if a file exists.
	FileExists(path string) bool
}

// VendorConfigReader is an interface for reading vendor configurations.
//
//go:generate mockgen -source=vendor_interfaces.go -destination=mock_vendor_interfaces.go -package=exec VendorConfigReader
type VendorConfigReader interface {
	// ReadVendorConfig reads and parses a vendor configuration file.
	ReadVendorConfig(path string) (schema.AtmosVendorConfig, error)
	// ReadComponentVendorConfig reads and parses a component vendor configuration file.
	ReadComponentVendorConfig(atmosConfig *schema.AtmosConfiguration, component, componentType string) (schema.VendorComponentConfig, string, error)
	// ProcessImports processes vendor config imports and returns all sources with file mappings.
	ProcessImports(atmosConfig *schema.AtmosConfiguration, vendorConfig schema.AtmosVendorConfig, configFile string) ([]schema.AtmosVendorSource, map[string]string, error)
}

// YAMLUpdater is an interface for updating YAML files while preserving structure.
//
//go:generate mockgen -source=vendor_interfaces.go -destination=mock_vendor_interfaces.go -package=exec YAMLUpdater
type YAMLUpdater interface {
	// UpdateVersionsInFile updates versions in a YAML file while preserving structure.
	UpdateVersionsInFile(filePath string, updates map[string]string) error
	// UpdateVersionsInContent updates versions in YAML content while preserving structure.
	UpdateVersionsInContent(content []byte, updates map[string]string) ([]byte, error)
}
