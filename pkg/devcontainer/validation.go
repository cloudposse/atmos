package devcontainer

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ValidateNotImported checks that devcontainers are not used as component dependencies.
// Devcontainers are workspace-level tools and should not be imported by components.
func ValidateNotImported(importPath string) error {
	defer perf.Track(nil, "devcontainer.ValidateNotImported")()

	// Check if the import path contains devcontainer configuration
	// This would indicate someone is trying to import a devcontainer as a component dependency
	if containsDevcontainerConfig(importPath) {
		return fmt.Errorf("%w: devcontainers cannot be used as component dependencies (path: %s)", errUtils.ErrInvalidDevcontainerConfig, importPath)
	}

	return nil
}

// containsDevcontainerConfig checks if a path contains devcontainer configuration.
func containsDevcontainerConfig(path string) bool {
	defer perf.Track(nil, "devcontainer.containsDevcontainerConfig")()

	// Check for common devcontainer file names anywhere in the path.
	devcontainerFiles := []string{
		"devcontainer.json",
		".devcontainer/devcontainer.json",
		".devcontainer.json",
	}

	for _, file := range devcontainerFiles {
		if strings.Contains(path, file) {
			return true
		}
	}

	return false
}
