package backend

import (
	"context"
	"fmt"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// BackendCreateFunc is a function that creates a Terraform backend.
type BackendCreateFunc func(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	backendConfig map[string]any,
	authContext *schema.AuthContext,
) error

// BackendDeleteFunc is a function that deletes a Terraform backend.
type BackendDeleteFunc func(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	backendConfig map[string]any,
	authContext *schema.AuthContext,
	force bool,
) error

var (
	// BackendCreators maps backend type (s3, gcs, azurerm) to create function.
	backendCreators = make(map[string]BackendCreateFunc)
	// BackendDeleters maps backend type (s3, gcs, azurerm) to delete function.
	backendDeleters = make(map[string]BackendDeleteFunc)
	registryMu      sync.RWMutex
)

// RegisterBackendCreate registers a backend create function for a specific backend type.
func RegisterBackendCreate(backendType string, fn BackendCreateFunc) {
	defer perf.Track(nil, "backend.RegisterBackendCreate")()

	registryMu.Lock()
	defer registryMu.Unlock()

	backendCreators[backendType] = fn
}

// GetBackendCreate returns the create function for a backend type.
// Returns nil if no create function is registered for the type.
func GetBackendCreate(backendType string) BackendCreateFunc {
	defer perf.Track(nil, "backend.GetBackendCreate")()

	registryMu.RLock()
	defer registryMu.RUnlock()

	return backendCreators[backendType]
}

// RegisterBackendDelete registers a backend delete function for a specific backend type.
func RegisterBackendDelete(backendType string, fn BackendDeleteFunc) {
	defer perf.Track(nil, "backend.RegisterBackendDelete")()

	registryMu.Lock()
	defer registryMu.Unlock()

	backendDeleters[backendType] = fn
}

// GetBackendDelete returns the delete function for a backend type.
// Returns nil if no delete function is registered for the type.
func GetBackendDelete(backendType string) BackendDeleteFunc {
	defer perf.Track(nil, "backend.GetBackendDelete")()

	registryMu.RLock()
	defer registryMu.RUnlock()

	return backendDeleters[backendType]
}

// ResetRegistryForTesting clears the backend provisioner registry.
// This function is intended for use in tests to ensure test isolation.
// It should be called via t.Cleanup() to restore clean state after each test.
func ResetRegistryForTesting() {
	defer perf.Track(nil, "backend.ResetRegistryForTesting")()

	registryMu.Lock()
	defer registryMu.Unlock()
	backendCreators = make(map[string]BackendCreateFunc)
	backendDeleters = make(map[string]BackendDeleteFunc)
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
		return fmt.Errorf("%w: backend configuration not found", errUtils.ErrBackendNotFound)
	}

	backendType, ok := componentConfig["backend_type"].(string)
	if !ok {
		return fmt.Errorf("%w: backend_type not specified", errUtils.ErrBackendTypeRequired)
	}

	// Get create function for backend type.
	createFunc := GetBackendCreate(backendType)
	if createFunc == nil {
		return fmt.Errorf("%w: %s", errUtils.ErrCreateNotImplemented, backendType)
	}

	// Execute create function.
	return createFunc(ctx, atmosConfig, backendConfig, authContext)
}
