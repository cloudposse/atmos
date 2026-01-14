package provisioner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/backend"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

// HookEventBeforeTerraformInit is the hook event for before terraform init.
const HookEventBeforeTerraformInit = HookEvent("before.terraform.init")

func init() {
	// Register backend provisioner to run before terraform init.
	// This enables automatic backend creation when provision.backend.enabled: true.
	_ = RegisterProvisioner(Provisioner{
		Type:      "backend",
		HookEvent: HookEventBeforeTerraformInit,
		Func:      autoProvisionBackend,
	})
}

// autoProvisionBackend automatically provisions the backend on terraform init.
//
// Behavior:
// - If provision.backend.enabled is not true: silent skip (not an error).
// - If backend already exists: silent skip (idempotent).
// - If backend doesn't exist: create with spinner feedback.
func autoProvisionBackend(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	authContext *schema.AuthContext,
) error {
	defer perf.Track(atmosConfig, "provisioner.autoProvisionBackend")()

	// Check if provisioning is enabled - silent skip if not.
	if !isBackendProvisionEnabled(componentConfig) {
		return nil
	}

	// Get backend configuration.
	backendConfig, ok := componentConfig["backend"].(map[string]any)
	if !ok {
		return nil // No backend configuration - skip.
	}

	backendType, ok := componentConfig["backend_type"].(string)
	if !ok {
		return nil // No backend type - skip.
	}

	// Get create function for backend type.
	createFunc := backend.GetBackendCreate(backendType)
	if createFunc == nil {
		return nil // Backend type not supported for auto-provisioning - skip.
	}

	// Check if backend already exists.
	exists, err := backend.BackendExists(ctx, atmosConfig, backendType, backendConfig, authContext)
	if err != nil {
		// Log error but don't fail - let terraform init surface the real error.
		return nil
	}
	if exists {
		return nil // Backend exists - silent skip.
	}

	// Get component, stack, and backend name for spinner message.
	component := extractBackendComponent(componentConfig)
	stack := extractBackendStack(componentConfig)
	backendName := backend.BackendName(backendType, backendConfig)

	// Execute backend provisioner with spinner feedback.
	progressMsg := fmt.Sprintf("Provisioning %s backend `%s` for `%s` in stack `%s`", strings.ToUpper(backendType), backendName, component, stack)
	completedMsg := fmt.Sprintf("Provisioned %s backend `%s` for `%s` in stack `%s`", strings.ToUpper(backendType), backendName, component, stack)

	return spinner.ExecWithSpinner(progressMsg, completedMsg, func() error {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()

		return createFunc(ctx, atmosConfig, backendConfig, authContext)
	})
}

// isBackendProvisionEnabled checks if provision.backend.enabled is true.
func isBackendProvisionEnabled(componentConfig map[string]any) bool {
	provision, ok := componentConfig["provision"].(map[string]any)
	if !ok {
		return false
	}

	backend, ok := provision["backend"].(map[string]any)
	if !ok {
		return false
	}

	enabled, ok := backend["enabled"].(bool)
	return ok && enabled
}

// extractBackendComponent extracts the component name from config.
func extractBackendComponent(componentConfig map[string]any) string {
	if component, ok := componentConfig["atmos_component"].(string); ok && component != "" {
		return component
	}
	if component, ok := componentConfig["component"].(string); ok && component != "" {
		return component
	}
	if metadata, ok := componentConfig["metadata"].(map[string]any); ok {
		if component, ok := metadata["component"].(string); ok && component != "" {
			return component
		}
	}
	return "unknown"
}

// extractBackendStack extracts the stack name from config.
func extractBackendStack(componentConfig map[string]any) string {
	if stack, ok := componentConfig["atmos_stack"].(string); ok && stack != "" {
		return stack
	}
	return "unknown"
}
