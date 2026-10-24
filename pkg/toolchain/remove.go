package toolchain

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
)

// RemoveToolVersion removes either the entire tool or a specific version from the file.
// If version is empty, removes all versions of the tool.
func RemoveToolVersion(filePath, tool, version string) error {
	defer perf.Track(nil, "toolchain.RemoveToolVersionFromFile")()

	if err := validateRemoveInput(tool); err != nil {
		return err
	}

	var result removeResult
	err := withToolVersionsLock(filePath, func() error {
		toolVersions, versions, err := loadToolVersionsForRemoval(filePath, tool)
		if err != nil {
			return err
		}
		if version == "" {
			removeAllVersions(toolVersions, tool)
			result = removeResult{tool: tool, removedAll: true}
		} else {
			newVersions, removed := removeSpecificVersion(versions, version)
			if !removed {
				return fmt.Errorf("%w: version '%s' not found for tool '%s' in %s", ErrNoVersionsFound, version, tool, filePath)
			}
			updateToolVersionsAfterRemoval(toolVersions, tool, newVersions)
			result = removeResult{tool: tool, version: version, removedAll: false}
		}
		return saveToolVersionsUnlocked(filePath, toolVersions)
	})
	if err != nil {
		return err
	}

	displayRemovalSuccess(result, filePath)
	return nil
}
