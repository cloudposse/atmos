package helm

import (
	"context"
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"

	// Blank import registers the "git" provision target kind so it is available
	// for delivery whenever Helm components are executed.
	_ "github.com/cloudposse/atmos/pkg/provisioner/target/git"
)

// ComponentProvider implements the component.ComponentProvider interface for
// native Helm components.
type ComponentProvider struct{}

// executeOperation is a seam so tests can stub execution.
var executeOperation = Execute

func init() {
	defer perf.Track(nil, "helm.init")()

	if err := component.Register(&ComponentProvider{}); err != nil {
		panic(fmt.Sprintf("failed to register helm component provider: %v", err))
	}
}

// GetType returns the component type identifier.
func (p *ComponentProvider) GetType() string {
	defer perf.Track(nil, "helm.GetType")()
	return cfg.HelmComponentType
}

// GetGroup returns the component group for categorization.
func (p *ComponentProvider) GetGroup() string {
	defer perf.Track(nil, "helm.GetGroup")()
	return "Kubernetes"
}

// GetBasePath returns the base directory for Helm components.
func (p *ComponentProvider) GetBasePath(atmosConfig *schema.AtmosConfiguration) string {
	defer perf.Track(atmosConfig, "helm.GetBasePath")()

	if atmosConfig == nil || atmosConfig.Components.Helm.BasePath == "" {
		return DefaultConfig().BasePath
	}
	return atmosConfig.Components.Helm.BasePath
}

// ListComponents discovers all Helm components in a stack.
func (p *ComponentProvider) ListComponents(_ context.Context, _ string, stackConfig map[string]any) ([]string, error) {
	defer perf.Track(nil, "helm.ListComponents")()

	componentsSection, ok := stackConfig["components"].(map[string]any)
	if !ok {
		return []string{}, nil
	}

	helmComponents, ok := componentsSection[cfg.HelmComponentType].(map[string]any)
	if !ok {
		return []string{}, nil
	}

	componentNames := make([]string, 0, len(helmComponents))
	for name := range helmComponents {
		componentNames = append(componentNames, name)
	}
	sort.Strings(componentNames)
	return componentNames, nil
}

// ValidateComponent validates Helm component configuration.
func (p *ComponentProvider) ValidateComponent(config map[string]any) error {
	defer perf.Track(nil, "helm.ValidateComponent")()

	if config == nil {
		return nil
	}

	if metadata, ok := config["metadata"].(map[string]any); ok {
		if componentType, ok := metadata["type"].(string); ok && componentType == "abstract" {
			return nil
		}
	}

	if chart, ok := config[cfg.ChartSectionName].(string); !ok || chart == "" {
		return errUtils.ErrHelmChartNotConfigured
	}

	return nil
}

// Execute runs the requested subcommand for a Helm component.
func (p *ComponentProvider) Execute(ctx *component.ExecutionContext) error {
	defer perf.Track(ctx.AtmosConfig, "helm.Execute")()

	switch ctx.SubCommand {
	case "template", "render":
		return executeOperation(ctx, OperationTemplate)
	case "diff", "plan":
		return executeOperation(ctx, OperationDiff)
	case "apply", "deploy":
		return executeOperation(ctx, OperationApply)
	case "delete", "destroy":
		return executeOperation(ctx, OperationDelete)
	default:
		return fmt.Errorf("%w: %q", errUtils.ErrHelmUnsupportedSubcommand, ctx.SubCommand)
	}
}

// GenerateArtifacts is a no-op for Helm components (charts are self-contained).
func (p *ComponentProvider) GenerateArtifacts(_ *component.ExecutionContext) error {
	defer perf.Track(nil, "helm.GenerateArtifacts")()
	return nil
}

// GetAvailableCommands returns the subcommands Helm components support.
func (p *ComponentProvider) GetAvailableCommands() []string {
	defer perf.Track(nil, "helm.GetAvailableCommands")()
	return []string{"template", "diff", "plan", "apply", "deploy", "delete"}
}
