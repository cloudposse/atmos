package toolchain

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// RemoveToolVersion removes either the entire tool or a specific version.
// Returns the version removed (empty if all versions were removed).
func RemoveToolVersion(filePath, tool, version string) error {
	defer perf.Track(nil, "toolchain.RemoveToolVersionFromFile")()

	if tool == "" {
		return fmt.Errorf("%w: empty tool argument", ErrInvalidToolSpec)
	}

	toolVersions, err := LoadToolVersions(filePath)
	if err != nil {
		return err
	}

	versions, exists := toolVersions.Tools[tool]
	if !exists {
		return fmt.Errorf("%w: tool '%s' not found in %s", ErrToolNotFound, tool, filePath)
	}

	if version == "" {
		// Remove all versions
		delete(toolVersions.Tools, tool)
		if err := SaveToolVersions(filePath, toolVersions); err != nil {
			return err
		}
		return nil
	}

	// Remove only the specified version
	newVersions := make([]string, 0, len(versions))
	removed := false
	for _, v := range versions {
		if v == version {
			removed = true
			continue
		}
		newVersions = append(newVersions, v)
	}
	if !removed {
		return fmt.Errorf("%w: version '%s' not found for tool '%s' in %s", ErrNoVersionsFound, version, tool, filePath)
	}

	if len(newVersions) == 0 {
		delete(toolVersions.Tools, tool)
	} else {
		toolVersions.Tools[tool] = newVersions
	}

	if err := SaveToolVersions(filePath, toolVersions); err != nil {
		return err
	}

	if version == "" {
		_ = ui.Successf("Removed %s from %s", tool, filePath)
	} else {
		_ = ui.Successf("Removed %s@%s from %s", tool, version, filePath)
	}
	return nil
}
