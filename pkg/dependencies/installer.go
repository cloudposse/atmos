package dependencies

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/toolchain"
	"github.com/cloudposse/atmos/toolchain/registry"
)

// Installer handles automatic tool installation.
type Installer struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewInstaller creates a new tool installer.
func NewInstaller(atmosConfig *schema.AtmosConfiguration) *Installer {
	defer perf.Track(nil, "dependencies.NewInstaller")()

	return &Installer{
		atmosConfig: atmosConfig,
	}
}

// EnsureTools ensures all required tools are installed.
// Installs missing tools automatically.
func (i *Installer) EnsureTools(dependencies map[string]string) error {
	defer perf.Track(i.atmosConfig, "dependencies.EnsureTools")()

	if len(dependencies) == 0 {
		return nil
	}

	for tool, version := range dependencies {
		if err := i.ensureTool(tool, version); err != nil {
			return err
		}
	}

	return nil
}

// ensureTool ensures a specific tool version is installed.
func (i *Installer) ensureTool(tool string, version string) error {
	defer perf.Track(i.atmosConfig, "dependencies.ensureTool")()

	// Check if already installed
	if i.isToolInstalled(tool, version) {
		return nil
	}

	// Install missing tool
	toolSpec := fmt.Sprintf("%s@%s", tool, version)
	if err := toolchain.RunInstall(toolSpec, false, false); err != nil {
		return fmt.Errorf("%w: failed to install %s: %w", errUtils.ErrToolInstall, toolSpec, err)
	}

	return nil
}

// isToolInstalled checks if a tool version is already installed.
func (i *Installer) isToolInstalled(tool string, version string) bool {
	defer perf.Track(i.atmosConfig, "dependencies.Installer.isToolInstalled")()

	// Get tools directory
	toolsDir := ".tools"
	if i.atmosConfig != nil && i.atmosConfig.Toolchain.InstallPath != "" {
		toolsDir = i.atmosConfig.Toolchain.InstallPath
	}

	// Resolve tool to owner/repo
	owner, repo, err := resolveToolPath(tool)
	if err != nil {
		return false
	}

	// Check if binary exists
	binaryPath := filepath.Join(toolsDir, "bin", owner, repo, version, repo)
	return fileExists(binaryPath)
}

// resolveToolPath resolves a tool name to owner/repo format.
// Handles both aliases (terraform) and full paths (hashicorp/terraform).
func resolveToolPath(tool string) (string, string, error) {
	// If already in owner/repo format
	if strings.Contains(tool, "/") {
		parts := strings.Split(tool, "/")
		if len(parts) != 2 {
			return "", "", fmt.Errorf("%w: %s", registry.ErrInvalidToolSpec, tool)
		}
		return parts[0], parts[1], nil
	}

	// Cannot resolve tool - return error instead of misleading "unknown" owner.
	// Callers should handle resolution errors appropriately.
	return "", "", fmt.Errorf("%w: unable to resolve tool '%s' to owner/repo", registry.ErrToolNotFound, tool)
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// UpdatePathForTools updates PATH environment variable to include tool binaries.
func UpdatePathForTools(atmosConfig *schema.AtmosConfiguration, dependencies map[string]string) error {
	defer perf.Track(atmosConfig, "dependencies.UpdatePathForTools")()

	if len(dependencies) == 0 {
		return nil
	}

	// Guard against nil atmosConfig.
	toolsDir := ".tools"
	if atmosConfig != nil && atmosConfig.Toolchain.InstallPath != "" {
		toolsDir = atmosConfig.Toolchain.InstallPath
	}

	var paths []string
	for tool, version := range dependencies {
		// Resolve tool to owner/repo
		owner, repo, err := resolveToolPath(tool)
		if err != nil {
			continue // Skip invalid tools
		}

		// Add versioned bin directory to PATH
		binPath := filepath.Join(toolsDir, "bin", owner, repo, version)
		paths = append(paths, binPath)
	}

	if len(paths) == 0 {
		return nil
	}

	// Prepend to existing PATH.
	currentPath := os.Getenv("PATH")
	newPath := strings.Join(append(paths, currentPath), string(os.PathListSeparator))
	os.Setenv("PATH", newPath)

	return nil
}
