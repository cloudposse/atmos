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

	entryNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		// A freshly `git init`'d directory has a `.git` entry but no real user
		// content; counting it would treat an otherwise-empty target as
		// non-empty and block generation without --force/--update.
		if entry.Name() == ".git" {
			continue
		}
		entryNames = append(entryNames, entry.Name())
	}

	// If force or update is enabled, or no files exist, allow the operation.
	if force || update || len(entryNames) == 0 {
		return nil
	}

	return errUtils.Build(errUtils.ErrTargetDirectoryNotEmpty).
		WithExplanationf("Directory `%s` already contains files", targetPath).
		WithExplanationf("Files: `%s`", strings.Join(entryNames, ", ")).
		WithHint("Use `--force` to overwrite existing files").
		WithHint("Use `--update` to merge changes via a 3-way merge instead").
		WithHint("Or choose a different target directory").
		WithContext("target_dir", targetPath).
		WithContext("file_count", len(entryNames)).
		WithContext("files", strings.Join(entryNames, ", ")).
		WithExitCode(2).
		Err()
}
