package output

import (
	"context"
	"os"
	"path/filepath"
	"strings"

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

	log.Debug("Selecting terraform workspace", "workspace", workspace, "component", component, "stack", stack)

	// Try to select existing workspace first (most common case: workspace already exists).
	// This order is more correct for the output-reading use case and avoids a Windows race
	// condition where WorkspaceNew succeeds (creating a NEW empty workspace) instead of
	// failing with "already exists" due to filesystem latency after init -reconfigure.
	err := runner.WorkspaceSelect(ctx, workspace)
	if err == nil {
		log.Debug("Successfully selected terraform workspace", "workspace", workspace, "component", component, "stack", stack)
		// Add delay on Windows after workspace operations.
		windowsFileDelay()
		return nil
	}

	// Only fall back to WorkspaceNew if the select error indicates a missing
	// workspace. Other errors (auth, backend, permission) should fail fast —
	// creating a new empty workspace would mask the real problem.
	if !isWorkspaceMissingError(err) {
		log.Debug("Workspace select failed with non-missing error", "workspace", workspace, "error", err)
		return wrapErrorWithStderr(
			errUtils.Build(errUtils.ErrTerraformWorkspaceOp).
				WithCause(err).
				WithExplanationf("Failed to select workspace '%s' for %s.", workspace, GetComponentInfo(component, stack)).
				Err(),
			stderrCapture,
		)
	}

	log.Debug("Workspace does not exist, creating it", "workspace", workspace, "component", component, "stack", stack)

	if err := runner.WorkspaceNew(ctx, workspace); err != nil {
		return wrapErrorWithStderr(
			errUtils.Build(errUtils.ErrTerraformWorkspaceOp).
				WithCause(err).
				WithExplanationf("Failed to create workspace '%s' for %s.", workspace, GetComponentInfo(component, stack)).
				Err(),
			stderrCapture,
		)
	}

	log.Debug("Successfully created terraform workspace", "workspace", workspace, "component", component, "stack", stack)
	// Add delay on Windows after workspace operations.
	windowsFileDelay()
	return nil
}

// isWorkspaceExistsError checks if the error indicates the workspace already exists.
// Terraform-exec doesn't provide typed errors, so we check the error message.
// Terraform CLI outputs "Workspace X already exists" when trying to create an existing workspace.
func isWorkspaceExistsError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "already exists")
}

// isWorkspaceMissingError checks if the error indicates the workspace does not exist.
// This is used to decide whether a failed WorkspaceSelect should fall back to WorkspaceNew.
// Only missing-workspace errors should trigger creation; other errors (auth, backend,
// permission) should fail fast to avoid masking real problems with a new empty workspace.
//
// Known error patterns:
//   - Terraform: `Workspace "foo" doesn't exist.`
//   - OpenTofu:  `workspace "foo" does not exist`
func isWorkspaceMissingError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "doesn't exist") ||
		strings.Contains(errMsg, "does not exist")
}
