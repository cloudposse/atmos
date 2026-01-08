package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// HookEventBeforeTerraformInit is the hook event for before terraform init.
const HookEventBeforeTerraformInit = provisioner.HookEvent("before.terraform.init")

// WorkdirPath is the standard workdir directory name.
const WorkdirPath = ".workdir"

// DirPermissions is the default permission mode for directories.
const DirPermissions = 0o755

func init() {
	// Register source provisioner to run before terraform init.
	// This enables JIT (Just-in-Time) source vendoring on first use.
	_ = provisioner.RegisterProvisioner(provisioner.Provisioner{
		Type:      "source",
		HookEvent: HookEventBeforeTerraformInit,
		Func:      autoProvisionSourceTerraform,
	})
}

// autoProvisionSourceTerraform is a wrapper for the hook system that always uses terraform as component type.
// This matches the HookEventBeforeTerraformInit event.
func autoProvisionSourceTerraform(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	authContext *schema.AuthContext,
) error {
	return AutoProvisionSource(ctx, atmosConfig, cfg.TerraformComponentType, componentConfig, authContext)
}

// AutoProvisionSource automatically vendors component source on first use.
// This enables JIT (Just-in-Time) vendoring - sources are fetched automatically
// when running terraform/helmfile/packer commands if the target directory doesn't exist.
//
// Parameters:
// - componentType: the type of component ("terraform", "helmfile", or "packer").
//
// Behavior:
// - If component has no source configured: skip (not an error).
// - If target directory already exists: skip (use CRUD commands for updates).
// - If workdir is enabled: download source directly to workdir path.
// - If workdir is NOT enabled: download source to component path.
func AutoProvisionSource(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	componentType string,
	componentConfig map[string]any,
	authContext *schema.AuthContext,
) error {
	defer perf.Track(atmosConfig, "source.AutoProvisionSource")()

	sourceSpec, component, err := extractSourceAndComponent(componentConfig)
	if err != nil {
		return err
	}
	if sourceSpec == nil {
		return nil // No source configured - skip.
	}

	targetDir, isWorkdir, err := determineSourceTargetDirectory(atmosConfig, componentType, component, componentConfig)
	if err != nil {
		return wrapProvisionError(err, "Failed to determine target directory", component)
	}

	// Skip if target exists - set workdir path if needed and return.
	if !needsProvisioning(targetDir) {
		if isWorkdir {
			componentConfig[workdir.WorkdirPathKey] = targetDir
		}
		return nil
	}

	// Vendor the source to target directory.
	if err := vendorToTarget(ctx, atmosConfig, sourceSpec, targetDir, component); err != nil {
		return err
	}

	// Set workdir path for terraform execution if applicable.
	if isWorkdir {
		componentConfig[workdir.WorkdirPathKey] = targetDir
	}
	return nil
}

// extractSourceAndComponent extracts source spec and component name from config.
func extractSourceAndComponent(componentConfig map[string]any) (*schema.VendorComponentSource, string, error) {
	sourceSpec, err := ExtractSource(componentConfig)
	if err != nil {
		return nil, "", errUtils.Build(errUtils.ErrSourceProvision).
			WithCause(err).
			WithExplanation("Invalid source configuration").
			Err()
	}
	if sourceSpec == nil {
		return nil, "", nil
	}

	component := extractComponentName(componentConfig)
	if component == "" {
		return nil, "", errUtils.Build(errUtils.ErrSourceProvision).
			WithExplanation("Component name not found in configuration").
			Err()
	}
	return sourceSpec, component, nil
}

// vendorToTarget creates the target directory and vendors the source.
func vendorToTarget(ctx context.Context, atmosConfig *schema.AtmosConfiguration, sourceSpec *schema.VendorComponentSource, targetDir, component string) error {
	_ = ui.Info(fmt.Sprintf("Auto-provisioning source for component '%s'", component))

	if err := os.MkdirAll(targetDir, DirPermissions); err != nil {
		return errUtils.Build(errUtils.ErrSourceProvision).
			WithCause(err).
			WithExplanation("Failed to create target directory").
			WithContext("path", targetDir).
			Err()
	}

	if err := VendorSource(ctx, atmosConfig, sourceSpec, targetDir); err != nil {
		return errUtils.Build(errUtils.ErrSourceProvision).
			WithCause(err).
			WithExplanation("Failed to auto-provision component source").
			WithContext("component", component).
			WithContext("source", sourceSpec.Uri).
			WithContext("target", targetDir).
			WithHint("Verify source URI is accessible and credentials are valid").
			Err()
	}

	_ = ui.Success(fmt.Sprintf("Auto-provisioned source to %s", targetDir))
	return nil
}

// wrapProvisionError wraps an error with provision context.
func wrapProvisionError(err error, explanation, component string) error {
	return errUtils.Build(errUtils.ErrSourceProvision).
		WithCause(err).
		WithExplanation(explanation).
		WithContext("component", component).
		Err()
}

// determineSourceTargetDirectory determines where to download the source.
// Returns the target directory path and whether it's a workdir path.
//
// Priority:
// 1. If workdir is enabled: .workdir/<componentType>/<stack>-<component>/.
// 2. Otherwise: components/<componentType>/<component>/.
func determineSourceTargetDirectory(
	atmosConfig *schema.AtmosConfiguration,
	componentType string,
	component string,
	componentConfig map[string]any,
) (string, bool, error) {
	basePath := atmosConfig.BasePath
	if basePath == "" {
		basePath = "."
	}

	// Check if workdir is enabled.
	if isWorkdirEnabled(componentConfig) {
		// Get stack name for workdir path.
		stack, _ := componentConfig["atmos_stack"].(string)
		if stack == "" {
			return "", false, errUtils.Build(errUtils.ErrSourceProvision).
				WithExplanation("Stack name required when workdir is enabled").
				WithHint("The 'atmos_stack' field is required for workdir provisioning").
				Err()
		}

		// Build workdir path: .workdir/<componentType>/<stack>-<component>/
		workdirName := fmt.Sprintf("%s-%s", stack, component)
		workdirPath := filepath.Join(basePath, WorkdirPath, componentType, workdirName)
		return workdirPath, true, nil
	}

	// No workdir - use standard component path.
	targetDir, err := DetermineTargetDirectory(atmosConfig, componentType, component, componentConfig)
	if err != nil {
		return "", false, err
	}
	return targetDir, false, nil
}

// isWorkdirEnabled checks if provision.workdir.enabled is set to true.
func isWorkdirEnabled(componentConfig map[string]any) bool {
	provisionConfig, ok := componentConfig["provision"].(map[string]any)
	if !ok {
		return false
	}

	workdirConfig, ok := provisionConfig["workdir"].(map[string]any)
	if !ok {
		return false
	}

	enabled, ok := workdirConfig["enabled"].(bool)
	return ok && enabled
}

// needsProvisioning checks if the target directory needs provisioning.
// Returns true if directory doesn't exist or is empty.
func needsProvisioning(targetDir string) bool {
	info, err := os.Stat(targetDir)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		return true // Error accessing, assume needs provisioning.
	}
	if !info.IsDir() {
		return true // Not a directory, needs provisioning.
	}

	// Check if directory is empty.
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return true
	}
	return len(entries) == 0
}

// extractComponentName extracts the component name from config.
// Priority: componentConfig["component"] > componentConfig["metadata"]["component"].
func extractComponentName(componentConfig map[string]any) string {
	// Try component field first (highest priority).
	if component, ok := componentConfig["component"].(string); ok && component != "" {
		return component
	}

	// Fall back to metadata.component.
	if metadata, ok := componentConfig["metadata"].(map[string]any); ok {
		if component, ok := metadata["component"].(string); ok && component != "" {
			return component
		}
	}

	return ""
}
