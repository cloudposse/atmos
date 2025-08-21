package toolchain

import (
	"fmt"
	"log"
)

// AddToolVersion contains the actual business logic for adding/updating a tool.
func AddToolVersion(tool, version string) error {
	installer := NewInstaller()

	owner, repo, err := installer.parseToolSpec(tool)
	if err != nil {
		return fmt.Errorf("failed to resolve tool '%s': %w", tool, err)
	}

	// Ensure the tool exists in the registry
	if _, err := installer.findTool(owner, repo, version); err != nil {
		return fmt.Errorf("tool '%s' not found in registry: %w", tool, err)
	}

	// Add the tool to the version file
	if err := AddToolToVersions(GetToolVersionsFilePath(), tool, version); err != nil {
		return err
	}
	log.Printf("%s Added/updated %s %s in %s\n", checkMark.Render(), tool, version, atmosConfig.Toolchain.FilePath)
	return nil
}
