package toolchain

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/ui"
)

// removeResult holds the result of removing a tool or version.
type removeResult struct {
	tool       string
	version    string
	removedAll bool
}

// validateRemoveInput validates the tool parameter is not empty.
func validateRemoveInput(tool string) error {
	if tool == "" {
		return fmt.Errorf("%w: empty tool argument", ErrInvalidToolSpec)
	}
	return nil
}

// loadToolVersionsForRemoval loads the tool versions file and validates tool exists.
func loadToolVersionsForRemoval(filePath, tool string) (*ToolVersions, []string, error) {
	toolVersions, err := LoadToolVersions(filePath)
	if err != nil {
		return nil, nil, err
	}

	versions, exists := toolVersions.Tools[tool]
	if !exists {
		return nil, nil, fmt.Errorf("%w: tool '%s' not found in %s", ErrToolNotFound, tool, filePath)
	}

	return toolVersions, versions, nil
}

// removeAllVersions removes all versions of a tool from the tool versions map.
func removeAllVersions(toolVersions *ToolVersions, tool string) {
	delete(toolVersions.Tools, tool)
}

// removeSpecificVersion removes a specific version from the versions list.
// Returns the updated list and whether the version was found.
func removeSpecificVersion(versions []string, targetVersion string) ([]string, bool) {
	newVersions := make([]string, 0, len(versions))
	removed := false

	for _, v := range versions {
		if v == targetVersion {
			removed = true
			continue
		}
		newVersions = append(newVersions, v)
	}

	return newVersions, removed
}

// updateToolVersionsAfterRemoval updates the tool versions map after removing a specific version.
// If no versions remain, removes the tool entirely.
func updateToolVersionsAfterRemoval(toolVersions *ToolVersions, tool string, newVersions []string) {
	if len(newVersions) == 0 {
		delete(toolVersions.Tools, tool)
	} else {
		toolVersions.Tools[tool] = newVersions
	}
}

// displayRemovalSuccess displays a success message based on what was removed.
func displayRemovalSuccess(result removeResult, filePath string) {
	if result.removedAll {
		_ = ui.Successf("Removed %s from %s", result.tool, filePath)
	} else {
		_ = ui.Successf("Removed %s@%s from %s", result.tool, result.version, filePath)
	}
}
