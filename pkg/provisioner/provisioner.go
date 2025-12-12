package provisioner

import (
	"context"
	"errors"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/backend"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Error types for provisioning operations.
var ErrUnsupportedProvisionerType = errors.New("unsupported provisioner type")

// ExecuteDescribeComponentFunc is a function that describes a component from a stack.
// This allows us to inject the describe component logic without circular dependencies.
type ExecuteDescribeComponentFunc func(
	component string,
	stack string,
) (map[string]any, error)

// ProvisionParams contains parameters for the Provision function.
type ProvisionParams struct {
	AtmosConfig       *schema.AtmosConfiguration
	ProvisionerType   string
	Component         string
	Stack             string
	DescribeComponent ExecuteDescribeComponentFunc
	AuthContext       *schema.AuthContext
}

// Provision provisions infrastructure resources using a params struct.
// It validates the provisioner type, loads component configuration, and executes the provisioner.
func ProvisionWithParams(params *ProvisionParams) error {
	defer perf.Track(nil, "provision.ProvisionWithParams")()

	if params == nil {
		return fmt.Errorf("%w: provision params", errUtils.ErrNilParam)
	}

	if params.DescribeComponent == nil {
		return fmt.Errorf("%w: DescribeComponent callback", errUtils.ErrNilParam)
	}

	_ = ui.Info(fmt.Sprintf("Provisioning %s '%s' in stack '%s'", params.ProvisionerType, params.Component, params.Stack))

	// Get component configuration from stack.
	componentConfig, err := params.DescribeComponent(params.Component, params.Stack)
	if err != nil {
		return fmt.Errorf("failed to describe component: %w", err)
	}

	// Validate provisioner type.
	if params.ProvisionerType != "backend" {
		return fmt.Errorf("%w: %s (supported: backend)", ErrUnsupportedProvisionerType, params.ProvisionerType)
	}

	// Execute backend provisioner.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Pass AuthContext from params directly to backend provisioner.
	// This enables in-process SDK calls with Atmos-managed credentials.
	// The AuthContext was populated by the command layer through InitConfigAndAuth,
	// which merges component-level auth with global auth and respects default identity settings.
	authContext := params.AuthContext

	err = backend.ProvisionBackend(ctx, params.AtmosConfig, componentConfig, authContext)
	if err != nil {
		return fmt.Errorf("backend provisioning failed: %w", err)
	}

	_ = ui.Success(fmt.Sprintf("Successfully provisioned %s '%s' in stack '%s'", params.ProvisionerType, params.Component, params.Stack))
	return nil
}

// ListBackends lists all backends in a stack.
func ListBackends(atmosConfig *schema.AtmosConfiguration, opts interface{}) error {
	defer perf.Track(atmosConfig, "provision.ListBackends")()

	return errUtils.Build(errUtils.ErrNotImplemented).
		WithExplanation("List backends functionality is not yet implemented").
		WithHint("This feature is planned for a future release").
		Err()
}

// DescribeBackend returns the backend configuration from stack.
func DescribeBackend(atmosConfig *schema.AtmosConfiguration, component string, opts interface{}) error {
	defer perf.Track(atmosConfig, "provision.DescribeBackend")()

	return errUtils.Build(errUtils.ErrNotImplemented).
		WithExplanation("Describe backend functionality is not yet implemented").
		WithHint("This feature is planned for a future release").
		WithContext("component", component).
		Err()
}

// DeleteBackendParams contains parameters for the DeleteBackend function.
type DeleteBackendParams struct {
	AtmosConfig       *schema.AtmosConfiguration
	Component         string
	Stack             string
	Force             bool
	DescribeComponent ExecuteDescribeComponentFunc
	AuthContext       *schema.AuthContext
}

// validateDeleteParams validates DeleteBackendParams and returns an error if invalid.
func validateDeleteParams(params *DeleteBackendParams) error {
	if params == nil {
		return errUtils.Build(errUtils.ErrNilParam).WithExplanation("delete backend params cannot be nil").Err()
	}
	if params.DescribeComponent == nil {
		return errUtils.Build(errUtils.ErrNilParam).WithExplanation("DescribeComponent callback cannot be nil").Err()
	}
	return nil
}

// getBackendConfigFromComponent extracts backend configuration from component config.
func getBackendConfigFromComponent(componentConfig map[string]any, component, stack string) (map[string]any, string, error) {
	backendConfig, ok := componentConfig["backend"].(map[string]any)
	if !ok {
		return nil, "", errUtils.Build(errUtils.ErrBackendNotFound).
			WithExplanation("Backend configuration not found in component").
			WithContext("component", component).WithContext("stack", stack).
			WithHint("Ensure the component has a 'backend' block configured").Err()
	}
	backendType, ok := componentConfig["backend_type"].(string)
	if !ok {
		return nil, "", errUtils.Build(errUtils.ErrBackendTypeRequired).
			WithExplanation("Backend type not specified in component configuration").
			WithContext("component", component).WithContext("stack", stack).
			WithHint("Add 'backend_type' (e.g., 's3', 'gcs', 'azurerm') to the component configuration").Err()
	}
	return backendConfig, backendType, nil
}

// DeleteBackendWithParams deletes a backend using a params struct.
func DeleteBackendWithParams(params *DeleteBackendParams) error {
	defer perf.Track(nil, "provision.DeleteBackend")()

	if err := validateDeleteParams(params); err != nil {
		return err
	}

	_ = ui.Info(fmt.Sprintf("Deleting backend for component '%s' in stack '%s'", params.Component, params.Stack))

	componentConfig, err := params.DescribeComponent(params.Component, params.Stack)
	if err != nil {
		return errUtils.Build(errUtils.ErrDescribeComponent).WithCause(err).
			WithExplanation("Failed to describe component").
			WithContext("component", params.Component).WithContext("stack", params.Stack).
			WithHint("Verify the component exists in the specified stack").Err()
	}

	backendConfig, backendType, err := getBackendConfigFromComponent(componentConfig, params.Component, params.Stack)
	if err != nil {
		return err
	}

	deleteFunc := backend.GetBackendDelete(backendType)
	if deleteFunc == nil {
		return errUtils.Build(errUtils.ErrDeleteNotImplemented).
			WithExplanation("Delete operation not implemented for backend type").
			WithContext("backend_type", backendType).
			WithHint("Supported backend types for deletion: s3").Err()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	return deleteFunc(ctx, params.AtmosConfig, backendConfig, params.AuthContext, params.Force)
}
