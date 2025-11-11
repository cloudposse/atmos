package filesystem

import (
	"os"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

// ValidateTargetDirectory checks if the target directory exists and validates the operation.
func ValidateTargetDirectory(targetPath string, force, update bool) error {
	// Check if target directory exists
	if _, err := os.Stat(targetPath); err == nil {
		// Directory exists, check if it has any files that would conflict
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

		// Filter out hidden files and directories
		var visibleEntries []string
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".") {
				visibleEntries = append(visibleEntries, entry.Name())
			}
		}

		if len(visibleEntries) > 0 {
			if !force && !update {
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
		}
	}

	return nil
}
