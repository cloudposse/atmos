package provisioner

import (
	"context"
	"errors"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
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
	AuthManager       auth.AuthManager
}

// Provision provisions infrastructure resources.
// It validates the provisioner type, loads component configuration, and executes the provisioner.
//
//revive:disable:argument-limit
func Provision(
	atmosConfig *schema.AtmosConfiguration,
	provisionerType string,
	component string,
	stack string,
	describeComponent ExecuteDescribeComponentFunc,
	authManager auth.AuthManager,
) error {
	//revive:enable:argument-limit
	defer perf.Track(atmosConfig, "provision.Provision")()

	return ProvisionWithParams(&ProvisionParams{
		AtmosConfig:       atmosConfig,
		ProvisionerType:   provisionerType,
		Component:         component,
		Stack:             stack,
		DescribeComponent: describeComponent,
		AuthManager:       authManager,
	})
}

// ProvisionWithParams provisions infrastructure resources using a params struct.
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

	// Create AuthContext from AuthManager if provided.
	// This allows manual `atmos provision backend` commands to benefit from Atmos-managed auth (--identity, SSO).
	// The AuthManager handles authentication and writes credentials to files, which the backend provisioner
	// can then use via the AWS SDK's standard credential chain.
	//
	// TODO: In the future, we should populate a schema.AuthContext and pass it to ProvisionBackend
	// to enable in-process SDK calls with Atmos-managed credentials. For now, passing nil causes
	// the provisioner to fall back to the standard AWS SDK credential chain, which will pick up
	// the credentials written by AuthManager.
	var authContext *schema.AuthContext
	if params.AuthManager != nil {
		// Authentication already happened in cmd/provision/provision.go via CreateAndAuthenticateManager.
		// Credentials are available in files, so AWS SDK will pick them up automatically.
		// For now, pass nil and rely on AWS SDK credential chain.
		authContext = nil
	}

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

	_ = ui.Info("Listing backends")
	_ = ui.Warning("List functionality not yet implemented")
	return nil
}

// DescribeBackend returns the backend configuration from stack.
func DescribeBackend(atmosConfig *schema.AtmosConfiguration, component string, opts interface{}) error {
	defer perf.Track(atmosConfig, "provision.DescribeBackend")()

	_ = ui.Info(fmt.Sprintf("Describing backend for component '%s'", component))
	_ = ui.Warning("Describe functionality not yet implemented")
	return nil
}

// DeleteBackend deletes a backend.
// It loads the component configuration, gets the appropriate backend deleter from the registry,
// and executes the deletion with the force flag.
//
//revive:disable:argument-limit
func DeleteBackend(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	force bool,
	describeComponent ExecuteDescribeComponentFunc,
	authManager auth.AuthManager,
) error {
	//revive:enable:argument-limit
	defer perf.Track(atmosConfig, "provision.DeleteBackend")()

	_ = ui.Info(fmt.Sprintf("Deleting backend for component '%s' in stack '%s'", component, stack))

	// Get component configuration from stack.
	componentConfig, err := describeComponent(component, stack)
	if err != nil {
		return fmt.Errorf("failed to describe component: %w", err)
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

	// Get delete function for backend type.
	deleteFunc := backend.GetBackendDelete(backendType)
	if deleteFunc == nil {
		return fmt.Errorf("%w: %s (supported: s3)", errUtils.ErrDeleteNotImplemented, backendType)
	}

	// Execute backend delete function.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Pass authentication context to backend delete function.
	var authContext *schema.AuthContext

	return deleteFunc(ctx, atmosConfig, backendConfig, authContext, force)
}
