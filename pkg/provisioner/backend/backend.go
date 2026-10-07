package backend

import (
	"context"
	"fmt"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ProvisionResult holds the result of a backend provisioning operation.
type ProvisionResult struct {
	Warnings []string // Warning messages to display after spinner completes.
}

// BackendCreateFunc is a function that creates a Terraform backend.
// Optional settings are supplied through CreateOption values; backend types ignore options that do not apply.
type BackendCreateFunc func(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	backendConfig map[string]any,
	authContext *schema.AuthContext,
	opts ...CreateOption,
) (*ProvisionResult, error)

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
	backendExistsCheckers = make(map[string]BackendExistsFunc)
	backendNameExtractors = make(map[string]BackendNameFunc)
}

// BackendExistsFunc is a function that checks if a backend exists.
type BackendExistsFunc func(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	backendConfig map[string]any,
	authContext *schema.AuthContext,
) (bool, error)

// BackendExistsCheckers maps backend type to exists check function.
var backendExistsCheckers = make(map[string]BackendExistsFunc)

// RegisterBackendExists registers a backend exists function for a specific backend type.
func RegisterBackendExists(backendType string, fn BackendExistsFunc) {
	defer perf.Track(nil, "backend.RegisterBackendExists")()

	registryMu.Lock()
	defer registryMu.Unlock()

	backendExistsCheckers[backendType] = fn
}

// GetBackendExists returns the exists function for a backend type.
// Returns nil if no exists function is registered for the type.
func GetBackendExists(backendType string) BackendExistsFunc {
	defer perf.Track(nil, "backend.GetBackendExists")()

	registryMu.RLock()
	defer registryMu.RUnlock()

	return backendExistsCheckers[backendType]
}

// BackendExists checks if a backend already exists.
// Returns (true, nil) if backend exists, (false, nil) if it doesn't exist,
// or (false, error) if the check fails.
func BackendExists(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	backendType string,
	backendConfig map[string]any,
	authContext *schema.AuthContext,
) (bool, error) {
	defer perf.Track(atmosConfig, "backend.BackendExists")()

	existsFunc := GetBackendExists(backendType)
	if existsFunc == nil {
		// If no exists checker is registered, assume backend doesn't exist.
		return false, nil
	}

	return existsFunc(ctx, atmosConfig, backendConfig, authContext)
}

// BackendNameFunc is a function that extracts the backend resource name from config.
// For S3, this returns the bucket name. For GCS, the bucket name. For Azure, the container name.
type BackendNameFunc func(backendConfig map[string]any) string

// BackendNameExtractors maps backend type to name extraction function.
var backendNameExtractors = make(map[string]BackendNameFunc)

// RegisterBackendName registers a backend name function for a specific backend type.
func RegisterBackendName(backendType string, fn BackendNameFunc) {
	defer perf.Track(nil, "backend.RegisterBackendName")()

	registryMu.Lock()
	defer registryMu.Unlock()

	backendNameExtractors[backendType] = fn
}

// GetBackendName returns the name function for a backend type.
// Returns nil if no name function is registered for the type.
func GetBackendName(backendType string) BackendNameFunc {
	defer perf.Track(nil, "backend.GetBackendName")()

	registryMu.RLock()
	defer registryMu.RUnlock()

	return backendNameExtractors[backendType]
}

// BackendName extracts the backend resource name from config.
// Returns "unknown" if no name extractor is registered or if extraction fails.
func BackendName(backendType string, backendConfig map[string]any) string {
	defer perf.Track(nil, "backend.BackendName")()

	nameFunc := GetBackendName(backendType)
	if nameFunc == nil {
		return "unknown"
	}

	name := nameFunc(backendConfig)
	if name == "" {
		return "unknown"
	}
	return name
}

// ProvisionBackend provisions a backend if provisioning is enabled.
// Returns ProvisionResult with any warnings, and an error if provisioning fails or no provisioner is registered.
func ProvisionBackend(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	authContext *schema.AuthContext,
) (*ProvisionResult, error) {
	defer perf.Track(atmosConfig, "backend.ProvisionBackend")()

	// Check if provisioning is enabled.
	provision, ok := componentConfig["provision"].(map[string]any)
	if !ok {
		return nil, errUtils.Build(errUtils.ErrProvisioningNotConfigured).
			WithExplanation("No 'provision' configuration found for this component").
			WithHint("Add 'provision.backend.enabled: true' to the component's stack configuration").
			Err()
	}

	backend, ok := provision["backend"].(map[string]any)
	if !ok {
		return nil, errUtils.Build(errUtils.ErrProvisioningNotConfigured).
			WithExplanation("No 'provision.backend' configuration found for this component").
			WithHint("Add 'provision.backend.enabled: true' to the component's stack configuration").
			Err()
	}

	enabled, ok := backend["enabled"].(bool)
	if !ok || !enabled {
		return nil, errUtils.Build(errUtils.ErrProvisioningNotConfigured).
			WithExplanation("Backend provisioning is not enabled for this component").
			WithHint("Set 'provision.backend.enabled: true' in the component's stack configuration").
			Err()
	}

	// Get backend configuration.
	backendConfig, ok := componentConfig["backend"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: backend configuration not found", errUtils.ErrBackendNotFound)
	}

	backendType, ok := componentConfig["backend_type"].(string)
	if !ok {
		return nil, fmt.Errorf("%w: backend_type not specified", errUtils.ErrBackendTypeRequired)
	}

	// Get create function for backend type.
	createFunc := GetBackendCreate(backendType)
	if createFunc == nil {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrCreateNotImplemented, backendType)
	}

	// Read optional create settings from provision.backend (not from the generated backend config).
	opts, err := CreateOptionsFromComponent(componentConfig, backendType)
	if err != nil {
		return nil, err
	}

	// Execute create function.
	return createFunc(ctx, atmosConfig, backendConfig, authContext, opts...)
}

// CreateOptionsFromComponent builds the create options for a component's backend from its
// provision.backend section. Every caller that invokes a BackendCreateFunc, including the
// automatic provisioning that runs on terraform init, uses it so the options stay consistent.
//
// Settings that apply to a single backend type are read only for that type, so an unrelated
// backend ignores them even when the value is malformed. The values are checked here, before
// any existence check or AWS call, so a bad value fails fast.
func CreateOptionsFromComponent(componentConfig map[string]any, backendType string) ([]CreateOption, error) {
	defer perf.Track(nil, "backend.CreateOptionsFromComponent")()

	if backendType != backendTypeS3 {
		return nil, nil
	}

	provision, _ := componentConfig["provision"].(map[string]any)
	provisionBackend, _ := provision["backend"].(map[string]any)

	raw, present := provisionBackend["bucket_namespace"]
	if !present || raw == nil {
		return nil, nil
	}

	namespace, ok := raw.(string)
	if !ok {
		return nil, errUtils.Build(errUtils.ErrInvalidBucketNamespace).
			WithExplanationf("Got value of type %T", raw).
			WithHint("Set 'provision.backend.bucket_namespace' to a string").
			Err()
	}

	if err := validateBucketNamespace(namespace); err != nil {
		return nil, err
	}

	return []CreateOption{WithBucketNamespace(namespace)}, nil
}
