// Package source provides just-in-time (JIT) vendoring of component sources
// from source configuration in stack manifests.
package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

// ProvisionParams contains parameters for source provisioning.
type ProvisionParams struct {
	AtmosConfig     *schema.AtmosConfiguration
	ComponentType   string // "terraform", "helmfile", etc.
	Component       string
	Stack           string
	ComponentConfig map[string]any
	AuthContext     *schema.AuthContext
	Force           bool // Force re-vendor even if already exists.
}

// Provision vendors a component source based on source configuration.
// It extracts the source spec from component config, resolves it, and vendors
// to the appropriate target directory.
func Provision(ctx context.Context, params *ProvisionParams) error {
	defer perf.Track(nil, "source.Provision")()

	if params == nil {
		return errUtils.Build(errUtils.ErrNilParam).
			WithExplanation("provision params cannot be nil").
			Err()
	}

	// Extract source from component config.
	sourceSpec, err := ExtractSource(params.ComponentConfig)
	if err != nil {
		// An actual error occurred (e.g., invalid source spec).
		return errUtils.Build(errUtils.ErrSourceProvision).
			WithCause(err).
			WithExplanation("Failed to extract source").
			WithContext("component", params.Component).
			Err()
	}

	// No source configured - this is not an error, just skip.
	if sourceSpec == nil {
		return nil
	}

	// Determine target directory.
	targetDir, err := DetermineTargetDirectory(params.AtmosConfig, params.ComponentType, params.Component, params.ComponentConfig)
	if err != nil {
		return errUtils.Build(errUtils.ErrSourceProvision).
			WithCause(err).
			WithExplanation("Failed to determine target directory").
			WithContext("component", params.Component).
			WithContext("stack", params.Stack).
			Err()
	}

	// Check if vendoring is needed.
	if !params.Force && !needsVendoring(targetDir) {
		_ = ui.Info(fmt.Sprintf("Component already exists at %s (use --force to re-vendor)", targetDir))
		return nil
	}

	// Vendor the source with spinner feedback.
	progressMsg := fmt.Sprintf("Vendoring %s from %s", params.Component, sourceSpec.Uri)
	completedMsg := fmt.Sprintf("Vendored %s to %s", params.Component, targetDir)
	err = spinner.ExecWithSpinner(progressMsg, completedMsg, func() error {
		return VendorSource(ctx, params.AtmosConfig, sourceSpec, targetDir)
	})
	if err != nil {
		return errUtils.Build(errUtils.ErrSourceProvision).
			WithCause(err).
			WithExplanation("Failed to vendor component source").
			WithContext("source", sourceSpec.Uri).
			WithContext("target", targetDir).
			WithContext("component", params.Component).
			WithContext("stack", params.Stack).
			WithHint("Verify source URI is accessible and credentials are valid").
			Err()
	}

	return nil
}

// needsVendoring checks if the target directory needs vendoring.
// Returns true if directory doesn't exist or is empty.
func needsVendoring(targetDir string) bool {
	info, err := os.Stat(targetDir)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		return true // Error accessing, assume needs vendoring.
	}
	if !info.IsDir() {
		return true // Not a directory, needs vendoring.
	}

	// Check if directory is empty.
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return true
	}
	return len(entries) == 0
}

// DetermineTargetDirectory determines where to vendor the component source.
// Priority:
// 1. Working_directory if specified in component config.
// 2. Default component path: <base_path>/<component>.
func DetermineTargetDirectory(
	atmosConfig *schema.AtmosConfiguration,
	componentType string,
	component string,
	componentConfig map[string]any,
) (string, error) {
	defer perf.Track(atmosConfig, "source.DetermineTargetDirectory")()

	// Check for working_directory override in metadata or settings.
	if workdir := getWorkingDirectoryOverride(componentConfig); workdir != "" {
		return workdir, nil
	}

	// Get component base path using resolved paths from atmosConfig.
	componentBasePath, err := resolveComponentBasePath(atmosConfig, componentType)
	if err != nil {
		return "", err
	}

	return filepath.Join(componentBasePath, component), nil
}

// getWorkingDirectoryOverride checks for working_directory in metadata or settings.
func getWorkingDirectoryOverride(componentConfig map[string]any) string {
	if metadata, ok := componentConfig["metadata"].(map[string]any); ok {
		if workdir, ok := metadata["working_directory"].(string); ok && workdir != "" {
			return workdir
		}
	}
	if settings, ok := componentConfig["settings"].(map[string]any); ok {
		if workdir, ok := settings["working_directory"].(string); ok && workdir != "" {
			return workdir
		}
	}
	return ""
}

// resolveComponentBasePath resolves the component base path for a given component type.
func resolveComponentBasePath(atmosConfig *schema.AtmosConfiguration, componentType string) (string, error) {
	if atmosConfig == nil {
		return "", errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanation("AtmosConfiguration is nil").
			Err()
	}

	// Use pre-resolved absolute path if available.
	absPath := getResolvedAbsPath(atmosConfig, componentType)
	if absPath != "" {
		return absPath, nil
	}

	// Fall back to building path from config.
	return buildComponentPath(atmosConfig, componentType)
}

// getResolvedAbsPath returns the pre-resolved absolute path for a component type.
func getResolvedAbsPath(atmosConfig *schema.AtmosConfiguration, componentType string) string {
	switch componentType {
	case "terraform":
		return atmosConfig.TerraformDirAbsolutePath
	case "helmfile":
		return atmosConfig.HelmfileDirAbsolutePath
	case "packer":
		return atmosConfig.PackerDirAbsolutePath
	default:
		return ""
	}
}

// buildComponentPath builds the component path from config base paths.
func buildComponentPath(atmosConfig *schema.AtmosConfiguration, componentType string) (string, error) {
	configBasePath := getComponentBasePath(atmosConfig, componentType)
	if configBasePath == "" {
		return "", errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanation("Component base path not configured").
			WithContext("component_type", componentType).
			Err()
	}

	// Use configured path directly if absolute.
	if filepath.IsAbs(configBasePath) {
		return configBasePath, nil
	}

	// Join with atmos base path.
	basePath := atmosConfig.BasePath
	if basePath == "" {
		basePath = "."
	}
	return filepath.Join(basePath, configBasePath), nil
}

// getComponentBasePath returns the base path for a component type.
func getComponentBasePath(atmosConfig *schema.AtmosConfiguration, componentType string) string {
	if atmosConfig == nil {
		return ""
	}

	switch componentType {
	case "terraform":
		return atmosConfig.Components.Terraform.BasePath
	case "helmfile":
		return atmosConfig.Components.Helmfile.BasePath
	case "packer":
		return atmosConfig.Components.Packer.BasePath
	default:
		return ""
	}
}
