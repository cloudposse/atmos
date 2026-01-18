package provisioner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/backend"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

// Error types for provisioning operations.
var ErrUnsupportedProvisionerType = errors.New("unsupported provisioner type")

// getAtmosConfigFromProvisionParams safely extracts AtmosConfig from ProvisionParams.
func getAtmosConfigFromProvisionParams(params *ProvisionParams) *schema.AtmosConfiguration {
	if params == nil {
		return nil
	}
	return params.AtmosConfig
}

// getAtmosConfigFromDeleteParams safely extracts AtmosConfig from DeleteBackendParams.
func getAtmosConfigFromDeleteParams(params *DeleteBackendParams) *schema.AtmosConfiguration {
	if params == nil {
		return nil
	}
	return params.AtmosConfig
}

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
	defer perf.Track(getAtmosConfigFromProvisionParams(params), "provisioner.ProvisionWithParams")()

	if params == nil {
		return fmt.Errorf("%w: provision params", errUtils.ErrNilParam)
	}

	if params.DescribeComponent == nil {
		return fmt.Errorf("%w: DescribeComponent callback", errUtils.ErrNilParam)
	}

	// Get component configuration from stack.
	componentConfig, err := params.DescribeComponent(params.Component, params.Stack)
	if err != nil {
		return fmt.Errorf("failed to describe component: %w", err)
	}

	// Validate provisioner type.
	if params.ProvisionerType != "backend" {
		return fmt.Errorf("%w: %s (supported: backend)", ErrUnsupportedProvisionerType, params.ProvisionerType)
	}

	// Extract backend type and name for display.
	backendType, _ := componentConfig["backend_type"].(string)
	if backendType == "" {
		backendType = "backend"
	}
	backendConfig, _ := componentConfig["backend"].(map[string]any)
	backendName := backend.BackendName(backendType, backendConfig)

	// Execute backend provisioner with spinner feedback.
	progressMsg := fmt.Sprintf("Provisioning %s backend `%s` for `%s` in stack `%s`", strings.ToUpper(backendType), backendName, params.Component, params.Stack)
	completedMsg := fmt.Sprintf("Provisioned %s backend `%s` for `%s` in stack `%s`", strings.ToUpper(backendType), backendName, params.Component, params.Stack)

	// Capture provisioning result to display warnings after spinner completes.
	// Warnings must be displayed AFTER the spinner to avoid concurrent output corruption.
	var result *backend.ProvisionResult
	err = spinner.ExecWithSpinner(progressMsg, completedMsg, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		// Pass AuthContext from params directly to backend provisioner.
		// This enables in-process SDK calls with Atmos-managed credentials.
		// The AuthContext was populated by the command layer through InitConfigAndAuth,
		// which merges component-level auth with global auth and respects default identity settings.
		var provErr error
		result, provErr = backend.ProvisionBackend(ctx, params.AtmosConfig, componentConfig, params.AuthContext)
		return provErr
	})
	if err != nil {
		return err
	}

	// Display warnings AFTER spinner completes to avoid concurrent output issues.
	// The spinner runs operations in a background goroutine while animating on stderr,
	// so any output during spinner execution would interleave and corrupt the display.
	if result != nil {
		for _, warning := range result.Warnings {
			ui.Warning(warning)
		}
	}

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
	defer perf.Track(getAtmosConfigFromDeleteParams(params), "provisioner.DeleteBackendWithParams")()

	if err := validateDeleteParams(params); err != nil {
		return err
	}

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

	// Extract backend name for display.
	backendName := backend.BackendName(backendType, backendConfig)

	// Execute backend deletion with spinner feedback.
	progressMsg := fmt.Sprintf("Deleting %s backend `%s` for `%s` in stack `%s`", strings.ToUpper(backendType), backendName, params.Component, params.Stack)
	completedMsg := fmt.Sprintf("Deleted %s backend `%s` for `%s` in stack `%s`", strings.ToUpper(backendType), backendName, params.Component, params.Stack)

	return spinner.ExecWithSpinner(progressMsg, completedMsg, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		return deleteFunc(ctx, params.AtmosConfig, backendConfig, params.AuthContext, params.Force)
	})
}
