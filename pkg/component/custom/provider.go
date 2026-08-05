package custom

import (
	"context"
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Provider implements ComponentProvider for custom command component types.
// Unlike built-in providers (terraform, helmfile, packer), custom providers are
// registered on-demand when a custom command with a component type is executed.
type Provider struct {
	typeName string
	basePath string
}

// NewProvider creates a new custom component provider.
func NewProvider(typeName, basePath string) *Provider {
	defer perf.Track(nil, "custom.NewProvider")()

	return &Provider{
		typeName: typeName,
		basePath: basePath,
	}
}

// GetType returns the component type identifier.
func (p *Provider) GetType() string {
	defer perf.Track(nil, "custom.GetType")()

	return p.typeName
}

// GetGroup returns the component group for categorization.
func (p *Provider) GetGroup() string {
	defer perf.Track(nil, "custom.GetGroup")()

	return "Custom"
}

// GetBasePath returns the base directory path for this component type.
func (p *Provider) GetBasePath(atmosConfig *schema.AtmosConfiguration) string {
	defer perf.Track(atmosConfig, "custom.GetBasePath")()

	if p.basePath != "" {
		return p.basePath
	}

	// Default to components/<type>.
	return fmt.Sprintf("components/%s", p.typeName)
}

// ListComponents discovers all components of this type in a stack.
func (p *Provider) ListComponents(ctx context.Context, stack string, stackConfig map[string]any) ([]string, error) {
	defer perf.Track(nil, "custom.ListComponents")()

	componentsSection, ok := stackConfig["components"].(map[string]any)
	if !ok {
		return []string{}, nil
	}

	typeComponents, ok := componentsSection[p.typeName].(map[string]any)
	if !ok {
		return []string{}, nil
	}

	componentNames := make([]string, 0, len(typeComponents))
	for name := range typeComponents {
		componentNames = append(componentNames, name)
	}

	sort.Strings(componentNames)
	return componentNames, nil
}

// ValidateComponent validates component configuration.
// Custom components have no specific validation requirements.
func (p *Provider) ValidateComponent(config map[string]any) error {
	defer perf.Track(nil, "custom.ValidateComponent")()

	return nil
}

// Execute runs a command for this component type.
// For custom components, execution is handled by the custom command steps,
// not by the provider. This is a no-op.
func (p *Provider) Execute(ctx *component.ExecutionContext) error {
	defer perf.Track(ctx.AtmosConfig, "custom.Execute")()

	return nil
}

// GenerateArtifacts creates necessary files for component execution.
// Custom components do not generate artifacts.
func (p *Provider) GenerateArtifacts(ctx *component.ExecutionContext) error {
	defer perf.Track(ctx.AtmosConfig, "custom.GenerateArtifacts")()

	return nil
}

// GetAvailableCommands returns list of commands this component type supports.
// Custom component commands are defined in the custom command configuration.
func (p *Provider) GetAvailableCommands() []string {
	defer perf.Track(nil, "custom.GetAvailableCommands")()

	return []string{}
}

// EnsureRegistered registers a custom component type if not already registered.
// This is called during custom command execution when component.type is specified.
// It is idempotent - calling it multiple times with the same type is safe.
func EnsureRegistered(typeName, basePath string) error {
	defer perf.Track(nil, "custom.EnsureRegistered")()

	if typeName == "" {
		return errUtils.ErrComponentTypeEmpty
	}

	// Check if already registered.
	if _, ok := component.GetProvider(typeName); ok {
		return nil
	}

	// Register the new provider.
	provider := NewProvider(typeName, basePath)
	if err := component.Register(provider); err != nil {
		return fmt.Errorf("%w: %s: %w", errUtils.ErrCustomComponentTypeRegistration, typeName, err)
	}

	return nil
}
