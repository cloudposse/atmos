package output

import (
	"context"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// WorkspaceManager handles terraform workspace operations.
type WorkspaceManager interface {
	// CleanWorkspace removes the .terraform/environment file to ensure clean workspace state.
	CleanWorkspace(atmosConfig *schema.AtmosConfiguration, componentPath string)
	// EnsureWorkspace creates or selects the specified workspace.
	EnsureWorkspace(ctx context.Context, runner TerraformRunner, workspace, backendType, component, stack string, stderrCapture *quietModeWriter) error
}

// defaultWorkspaceManager is the default implementation of WorkspaceManager.
type defaultWorkspaceManager struct{}

// CleanWorkspace removes the .terraform/environment file to ensure clean workspace state.
func (m *defaultWorkspaceManager) CleanWorkspace(atmosConfig *schema.AtmosConfiguration, componentPath string) {
	defer perf.Track(atmosConfig, "output.defaultWorkspaceManager.CleanWorkspace")()

	// Get TF_DATA_DIR env variable, default to .terraform if not set.
	tfDataDir, ok := os.LookupEnv("TF_DATA_DIR")
	if !ok || tfDataDir == "" {
		tfDataDir = ".terraform"
	}

	// Remove the environment file.
	envFile := filepath.Join(componentPath, tfDataDir, "environment")
	if err := os.Remove(envFile); err != nil && !os.IsNotExist(err) {
		log.Debug("Could not remove terraform environment file", "file", envFile, "error", err)
	}
}

// EnsureWorkspace creates or selects the specified workspace.
// For HTTP backend, workspaces are not used so this is a no-op.
//
//nolint:revive // argument-limit: workspace operations require multiple context parameters.
func (m *defaultWorkspaceManager) EnsureWorkspace(
	ctx context.Context,
	runner TerraformRunner,
	workspace, backendType, component, stack string,
	stderrCapture *quietModeWriter,
) error {
	defer perf.Track(nil, "output.defaultWorkspaceManager.EnsureWorkspace")()

	// HTTP backend doesn't support workspaces.
	if backendType == "http" {
		log.Debug("Skipping workspace for HTTP backend", "component", component, "stack", stack)
		return nil
	}

	log.Debug("Creating terraform workspace", "workspace", workspace, "component", component, "stack", stack)

	// Try to create new workspace.
	err := runner.WorkspaceNew(ctx, workspace)
	if err == nil {
		log.Debug("Successfully created terraform workspace", "workspace", workspace, "component", component, "stack", stack)
		// Add delay on Windows after workspace operations.
		windowsFileDelay()
		return nil
	}

	// Log the creation failure before attempting select.
	log.Debug("Workspace creation failed, attempting select", "workspace", workspace, "error", err)

	// Workspace already exists, select it.
	log.Debug("Selecting existing terraform workspace", "workspace", workspace, "component", component, "stack", stack)

	if err := runner.WorkspaceSelect(ctx, workspace); err != nil {
		return wrapErrorWithStderr(
			errUtils.Build(errUtils.ErrTerraformWorkspaceOp).
				WithCause(err).
				WithExplanationf("Failed to select workspace '%s' for %s.", workspace, GetComponentInfo(component, stack)).
				Err(),
			stderrCapture,
		)
	}

	log.Debug("Successfully selected terraform workspace", "workspace", workspace, "component", component, "stack", stack)
	// Add delay on Windows after workspace operations.
	windowsFileDelay()
	return nil
}
