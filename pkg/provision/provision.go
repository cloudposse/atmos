package provision

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
//nolint:lintroller // This is a wrapper function that delegates to ProvisionWithParams, which has perf tracking.
func Provision(
	atmosConfig *schema.AtmosConfiguration,
	provisionerType string,
	component string,
	stack string,
	describeComponent ExecuteDescribeComponentFunc,
	authManager auth.AuthManager,
) error {
	//revive:enable:argument-limit
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
//
//nolint:lintroller // Perf tracking is added after nil check to avoid dereferencing nil params.
func ProvisionWithParams(params *ProvisionParams) error {
	// Note: We validate params before calling perf.Track to avoid nil pointer dereference.
	// The perf tracking is added after validation.
	if params == nil {
		return fmt.Errorf("%w: provision params", errUtils.ErrNilParam)
	}

	defer perf.Track(params.AtmosConfig, "provision.ProvisionWithParams")()

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
