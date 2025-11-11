package devcontainer

import (
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
		return errUtils.Build(errUtils.ErrInvalidDevcontainerConfig).
			WithExplanation("Devcontainers cannot be used as component dependencies").
			WithExplanationf("Import path `%s` contains devcontainer configuration", importPath).
			WithHint("Devcontainers are workspace-level development environments, not component dependencies").
			WithHint("Remove the devcontainer import from your component configuration").
			WithHint("Define devcontainers in the top-level `components.devcontainer` section of `atmos.yaml`").
			WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/configuration/").
			WithExample(`Correct usage:
# atmos.yaml
components:
  devcontainer:
    my-dev:  # Define devcontainer here
      spec:
        image: golang:latest

  terraform:
    vpc:
      # Don't import devcontainer here
      # imports: ...`).
			WithContext("import_path", importPath).
			WithExitCode(2).
			Err()
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
