package provision

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// Provision provisions infrastructure resources.
// It validates the provisioner type, loads component configuration, and executes the provisioner.
func Provision(
	atmosConfig *schema.AtmosConfiguration,
	provisionerType string,
	component string,
	stack string,
	describeComponent ExecuteDescribeComponentFunc,
) error {
	defer perf.Track(atmosConfig, "provision.Provision")()

	_ = ui.Info(fmt.Sprintf("Provisioning %s '%s' in stack '%s'", provisionerType, component, stack))

	// Get component configuration from stack.
	componentConfig, err := describeComponent(component, stack)
	if err != nil {
		return fmt.Errorf("failed to describe component: %w", err)
	}

	// Validate provisioner type.
	if provisionerType != "backend" {
		return fmt.Errorf("%w: %s (supported: backend)", ErrUnsupportedProvisionerType, provisionerType)
	}

	// Execute backend provisioner.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// AuthContext is not available in this context (no identity flag passed).
	// Provisioner will fall back to standard AWS SDK credential chain.
	err = backend.ProvisionBackend(ctx, atmosConfig, componentConfig, nil)
	if err != nil {
		return fmt.Errorf("backend provisioning failed: %w", err)
	}

	_ = ui.Success(fmt.Sprintf("Successfully provisioned %s '%s' in stack '%s'", provisionerType, component, stack))
	return nil
}
