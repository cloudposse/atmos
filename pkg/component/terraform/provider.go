// Package terraform implements the Terraform component provider. Host services
// are injected so the provider never depends on internal/exec.
package terraform

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_runtime_test.go -package=$GOPACKAGE

import (
	"context"
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Runtime supplies the existing Terraform lifecycle during its incremental
// extraction. Implementations must preserve execution options and error identity.
type Runtime interface {
	Execute(*component.ExecutionContext) error
	GenerateArtifacts(*component.ExecutionContext) error
}

// Provider is the built-in Terraform component implementation. Registration is
// performed by the host that supplies Runtime; importing this package has no
// registration side effects and cannot overwrite an installed provider.
type Provider struct{ runtime Runtime }

var _ component.ComponentProvider = (*Provider)(nil)

// New constructs a provider with host lifecycle services.
func New(runtime Runtime) *Provider {
	defer perf.Track(nil, "terraform.New")()

	return &Provider{runtime: runtime}
}

// GetType returns the stable component family, including OpenTofu executions.
func (*Provider) GetType() string {
	defer perf.Track(nil, "terraform.Provider.GetType")()

	return "terraform"
}

// GetGroup returns the infrastructure tooling group.
func (*Provider) GetGroup() string {
	defer perf.Track(nil, "terraform.Provider.GetGroup")()

	return "Infrastructure as Code"
}

// GetBasePath returns the configured source directory or its conventional default.
func (*Provider) GetBasePath(config *schema.AtmosConfiguration) string {
	defer perf.Track(nil, "terraform.Provider.GetBasePath")()

	if config != nil && config.Components.Terraform.BasePath != "" {
		return config.Components.Terraform.BasePath
	}
	return "components/terraform"
}

// ListComponents lists configured Terraform instances without evaluating them.
func (*Provider) ListComponents(ctx context.Context, _ string, stack map[string]any) ([]string, error) {
	defer perf.Track(nil, "terraform.Provider.ListComponents")()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	components, _ := stack["components"].(map[string]any)
	terraform, _ := components["terraform"].(map[string]any)
	names := make([]string, 0, len(terraform))
	for name := range terraform {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// ValidateComponent validates the resolved backend type's shape. Policy and
// module validation remain in the lifecycle, where stack and auth are available.
func (*Provider) ValidateComponent(config map[string]any) error {
	defer perf.Track(nil, "terraform.Provider.ValidateComponent")()

	if value, exists := config["backend_type"]; exists && value != nil {
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%w: backend_type must be a string", errUtils.ErrComponentConfigInvalid)
		}
	}
	return nil
}

// Execute runs a Terraform operation using the host's lifecycle services.
func (p *Provider) Execute(ctx *component.ExecutionContext) error {
	defer perf.Track(nil, "terraform.Provider.Execute")()

	if err := p.checkContext(ctx); err != nil {
		return err
	}
	return p.runtime.Execute(ctx)
}

// GenerateArtifacts writes files for an already resolved component. It does not
// initialize Terraform, select a workspace, or execute a plan/apply operation.
func (p *Provider) GenerateArtifacts(ctx *component.ExecutionContext) error {
	defer perf.Track(nil, "terraform.Provider.GenerateArtifacts")()

	if err := p.checkContext(ctx); err != nil {
		return err
	}
	if ctx.AtmosConfig == nil || ctx.WorkingDir == "" {
		return fmt.Errorf("%w: Terraform artifact generation requires configuration and a working directory", errUtils.ErrComponentConfigInvalid)
	}
	return p.runtime.GenerateArtifacts(ctx)
}

func (p *Provider) checkContext(ctx *component.ExecutionContext) error {
	if ctx == nil {
		return fmt.Errorf("%w: missing Terraform execution context", errUtils.ErrComponentConfigInvalid)
	}
	if err := ctx.GoContext().Err(); err != nil {
		return err
	}
	if p.runtime == nil {
		return fmt.Errorf("%w: Terraform runtime is not configured", errUtils.ErrComponentExecutionFailed)
	}
	return nil
}

// GetAvailableCommands advertises tool commands. Execution deliberately remains
// open to passthrough commands supported by the selected Terraform/OpenTofu binary.
func (*Provider) GetAvailableCommands() []string {
	defer perf.Track(nil, "terraform.Provider.GetAvailableCommands")()

	return []string{"apply", "console", "deploy", "destroy", "fmt", "force-unlock", "get", "graph", "import", "init", "login", "logout", "metadata", "output", "plan", "providers", "refresh", "show", "state", "taint", "test", "untaint", "validate", "version", "workspace"}
}
