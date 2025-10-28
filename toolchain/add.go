package toolchain

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// AddToolVersion contains the actual business logic for adding/updating a tool.
func AddToolVersion(tool, version string) error {
	defer perf.Track(nil, "toolchain.AddToolVersion")()

	installer := NewInstaller()

	owner, repo, err := installer.parseToolSpec(tool)
	if err != nil {
		return fmt.Errorf("failed to resolve tool '%s': %w", tool, err)
	}

	// Ensure the tool exists in the registry.
	if _, err := installer.findTool(owner, repo, version); err != nil {
		return fmt.Errorf("tool '%s' not found in registry: %w", tool, err)
	}

	// Add the tool to the version file.
	if err := AddToolToVersions(GetToolVersionsFilePath(), tool, version); err != nil {
		return err
	}
	u.PrintfMarkdownToTUI("%s Added/updated **%s** %s in %s\n", theme.Styles.Checkmark, tool, version, atmosConfig.Toolchain.VersionsFile)
	return nil
}
