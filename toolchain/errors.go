package toolchain

import "github.com/cloudposse/atmos/toolchain/registry"

// Error re-exports from the registry package for backward compatibility.
// New code should import and use toolchain/registry errors directly.
var (
	// ErrToolNotFound indicates a tool was not found in the registry or local configuration.
	ErrToolNotFound = registry.ErrToolNotFound

	// ErrNoVersionsFound indicates no versions are available for a tool.
	ErrNoVersionsFound = registry.ErrNoVersionsFound

	// ErrInvalidToolSpec indicates the tool specification format is invalid.
	ErrInvalidToolSpec = registry.ErrInvalidToolSpec

	// ErrHTTPRequest indicates an HTTP request failed.
	ErrHTTPRequest = registry.ErrHTTPRequest

	// ErrHTTP404 indicates an HTTP 404 Not Found response.
	ErrHTTP404 = registry.ErrHTTP404

	// ErrRegistryParse indicates the registry file could not be parsed.
	ErrRegistryParse = registry.ErrRegistryParse

	// ErrNoPackagesInRegistry indicates the registry contains no packages.
	ErrNoPackagesInRegistry = registry.ErrNoPackagesInRegistry

	// ErrNoAssetTemplate indicates no asset template is defined for the tool.
	ErrNoAssetTemplate = registry.ErrNoAssetTemplate

	// ErrFileOperation indicates a file operation failed.
	ErrFileOperation = registry.ErrFileOperation

	// ErrToolAlreadyExists indicates the tool version already exists in .tool-versions.
	ErrToolAlreadyExists = registry.ErrToolAlreadyExists
)
