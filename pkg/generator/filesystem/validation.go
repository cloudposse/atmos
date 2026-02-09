package filesystem

import (
	"errors"
	"os"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

// ValidateTargetDirectory checks if the target directory exists and validates the operation.
func ValidateTargetDirectory(targetPath string, force, update bool) error {
	// Check if target directory exists.
	_, err := os.Stat(targetPath)
	if errors.Is(err, os.ErrNotExist) {
		// Directory doesn't exist, nothing to validate.
		return nil
	}
	if err != nil {
		// Other error accessing the path.
		return err
	}

	// Directory exists, check for conflicts.
	return validateExistingDirectory(targetPath, force, update)
}

// validateExistingDirectory checks for conflicts in an existing directory.
func validateExistingDirectory(targetPath string, force, update bool) error {
	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return errUtils.Build(errUtils.ErrReadTargetDirectory).
			WithExplanationf("Cannot read directory: `%s`", targetPath).
			WithHint("Check directory permissions").
			WithHint("Verify the path is a valid directory").
			WithContext("target_dir", targetPath).
			WithExitCode(2).
			Err()
	}

	// Filter out hidden files and directories.
	visibleEntries := filterVisibleEntries(entries)

	// If force or update is enabled, or no visible files exist, allow the operation.
	if force || update || len(visibleEntries) == 0 {
		return nil
	}

	return errUtils.Build(errUtils.ErrTargetDirectoryNotEmpty).
		WithExplanationf("Directory `%s` already contains files", targetPath).
		WithExplanationf("Files: `%s`", strings.Join(visibleEntries, ", ")).
		WithHint("Use `--force` to overwrite existing files").
		WithHint("Use `--update` to merge changes with existing files").
		WithHint("Or choose a different target directory").
		WithContext("target_dir", targetPath).
		WithContext("file_count", len(visibleEntries)).
		WithContext("files", strings.Join(visibleEntries, ", ")).
		WithExitCode(2).
		Err()
}

// filterVisibleEntries returns only non-hidden directory entries.
func filterVisibleEntries(entries []os.DirEntry) []string {
	var visible []string
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".") {
			visible = append(visible, entry.Name())
		}
	}
	return visible
}
