package toolchain

import "errors"

// Error definitions for the toolchain package.
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
