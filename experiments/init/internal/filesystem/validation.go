package filesystem

import (
	"fmt"
	"os"
	"strings"
)

// ValidateTargetDirectory checks if the target directory exists and validates the operation
func ValidateTargetDirectory(targetPath string, force, update bool) error {
	// Check if target directory exists
	if _, err := os.Stat(targetPath); err == nil {
		// Directory exists, check if it has any files that would conflict
		entries, err := os.ReadDir(targetPath)
		if err != nil {
			return fmt.Errorf("failed to read target directory: %w", err)
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
				return fmt.Errorf("target directory '%s' already exists and contains files: %s (use --force to overwrite or --update to merge)",
					targetPath, strings.Join(visibleEntries, ", "))
			}
		}
	}

	return nil
}
