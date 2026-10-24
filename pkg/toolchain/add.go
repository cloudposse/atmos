package toolchain

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// AddToolVersion contains the actual business logic for adding/updating a tool.
// If setAsDefault is true, version replaces the tool's default (first) .tool-versions
// entry instead of being appended alongside any existing versions.
func AddToolVersion(tool, version string, setAsDefault bool) error {
	defer perf.Track(nil, "toolchain.AddToolVersion")()

	installer := NewInstaller()

	owner, repo, err := installer.ParseToolSpec(tool)
	if err != nil {
		return fmt.Errorf("failed to resolve tool '%s': %w", tool, err)
	}

	if err := ValidateVersionSpec(version); err != nil {
		return err
	}

	// Ensure the tool exists in the registry.
	if _, err := installer.FindTool(owner, repo, version); err != nil {
		return fmt.Errorf("tool '%s' not found in registry: %w", tool, err)
	}

	// Add the tool to the version file.
	filePath := GetToolVersionsFilePath()
	if err := updateToolVersionsFile(tool, version, setAsDefault); err != nil {
		return err
	}

	ui.Successf("Added/updated %s %s in %s", tool, version, filePath)
	return nil
}
