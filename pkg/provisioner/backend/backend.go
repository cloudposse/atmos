package backend

import (
	"context"
	"fmt"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

func init() {
	// Register backend provisioner to run before Terraform initialization.
	// This ensures the backend exists before Terraform tries to configure it.
	provisioner.RegisterProvisioner(provisioner.Provisioner{
		Type:      "backend",
		HookEvent: "before.terraform.init",
		Func:      ProvisionBackend,
	})
}

// BackendProvisionerFunc is a function that provisions a Terraform backend.
type BackendProvisionerFunc func(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	backendConfig map[string]any,
	authContext *schema.AuthContext,
) error

var (
	// BackendProvisioners maps backend type (s3, gcs, azurerm) to provisioner function.
	backendProvisioners = make(map[string]BackendProvisionerFunc)
	registryMu          sync.RWMutex
)

// RegisterBackendProvisioner registers a backend provisioner for a specific backend type.
func RegisterBackendProvisioner(backendType string, fn BackendProvisionerFunc) {
	defer perf.Track(nil, "backend.RegisterBackendProvisioner")()

	registryMu.Lock()
	defer registryMu.Unlock()

	backendProvisioners[backendType] = fn
}

// GetBackendProvisioner returns the provisioner function for a backend type.
// Returns nil if no provisioner is registered for the type.
func GetBackendProvisioner(backendType string) BackendProvisionerFunc {
	defer perf.Track(nil, "backend.GetBackendProvisioner")()

	registryMu.RLock()
	defer registryMu.RUnlock()

	return backendProvisioners[backendType]
}

// ProvisionBackend provisions a backend if provisioning is enabled.
// Returns an error if provisioning fails or no provisioner is registered.
func ProvisionBackend(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	authContext *schema.AuthContext,
) error {
	defer perf.Track(atmosConfig, "backend.ProvisionBackend")()

	// Check if provisioning is enabled.
	provision, ok := componentConfig["provision"].(map[string]any)
	if !ok {
		return nil // No provisioning configuration
	}

	backend, ok := provision["backend"].(map[string]any)
	if !ok {
		return nil // No backend provisioning configuration
	}

	enabled, ok := backend["enabled"].(bool)
	if !ok || !enabled {
		return nil // Provisioning not enabled
	}

	// Get backend configuration.
	backendConfig, ok := componentConfig["backend"].(map[string]any)
	if !ok {
		return fmt.Errorf("%w: backend configuration not found", provisioner.ErrBackendNotFound)
	}

	backendType, ok := componentConfig["backend_type"].(string)
	if !ok {
		return fmt.Errorf("%w: backend_type not specified", provisioner.ErrBackendTypeRequired)
	}

	// Get provisioner for backend type.
	prov := GetBackendProvisioner(backendType)
	if prov == nil {
		return fmt.Errorf("%w: %s", provisioner.ErrNoProvisionerFound, backendType)
	}

	// Execute provisioner.
	return prov(ctx, atmosConfig, backendConfig, authContext)
}
