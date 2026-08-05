package imports

import (
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ProcessImportPath resolves an import path (local or remote) to a local file path.
// For local paths, it joins with basePath.
// For remote URLs, it downloads and returns the temp file path.
func ProcessImportPath(atmosConfig *schema.AtmosConfiguration, basePath, importPath string) (string, error) {
	defer perf.Track(atmosConfig, "imports.ProcessImportPath")()

	// Check if the import path is a remote URL.
	if IsRemote(importPath) {
		// Download the remote import and return the local path.
		return DownloadRemoteImport(atmosConfig, importPath)
	}

	// Local path - join with basePath.
	return filepath.Join(basePath, importPath), nil
}

// ResolveImportPaths resolves multiple import paths, returning local file paths.
// This is a convenience function for batch processing.
func ResolveImportPaths(atmosConfig *schema.AtmosConfiguration, basePath string, importPaths []string) ([]string, error) {
	defer perf.Track(atmosConfig, "imports.ResolveImportPaths")()

	resolved := make([]string, len(importPaths))
	for i, importPath := range importPaths {
		path, err := ProcessImportPath(atmosConfig, basePath, importPath)
		if err != nil {
			return nil, err
		}
		resolved[i] = path
	}
	return resolved, nil
}
