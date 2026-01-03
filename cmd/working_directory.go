package cmd

import (
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
)

// resolveWorkingDirectory resolves and validates a working directory path.
// If workDir is empty, returns defaultDir. If workDir is relative, resolves against basePath.
// Returns error if the directory doesn't exist or isn't a directory.
func resolveWorkingDirectory(workDir, basePath, defaultDir string) (string, error) {
	if workDir == "" {
		return defaultDir, nil
	}

	// Clean and resolve paths. filepath.Clean normalizes paths like /tmp/foo/.. to /tmp.
	// For relative paths, filepath.Join already cleans the result.
	resolvedDir := filepath.Clean(workDir)
	if !filepath.IsAbs(workDir) {
		resolvedDir = filepath.Join(basePath, workDir)
	}

	// Validate directory exists and is a directory.
	info, err := os.Stat(resolvedDir)
	if os.IsNotExist(err) {
		return "", errUtils.Build(errUtils.ErrWorkingDirNotFound).
			WithCause(err).
			WithContext("path", resolvedDir).
			WithHint("Check that the working_directory path exists").
			Err()
	}
	if err != nil {
		return "", errUtils.Build(errUtils.ErrWorkingDirAccessFailed).
			WithCause(err).
			WithContext("path", resolvedDir).
			Err()
	}
	if !info.IsDir() {
		return "", errUtils.Build(errUtils.ErrWorkingDirNotDirectory).
			WithContext("path", resolvedDir).
			WithHint("The working_directory must be a directory, not a file").
			Err()
	}

	return resolvedDir, nil
}
