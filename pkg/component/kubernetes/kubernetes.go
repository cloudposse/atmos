package kubernetes

import (
	"context"
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ComponentProvider implements the Atmos component.ComponentProvider interface for
// the native Kubernetes component type (render/diff/apply/deploy/delete).
type ComponentProvider struct{}

var executeOperation = Execute

func init() {
	defer perf.Track(nil, "kubernetes.init")()

	if err := component.Register(&ComponentProvider{}); err != nil {
		panic(fmt.Sprintf("failed to register kubernetes component provider: %v", err))
	}
}

// GetType returns the component type identifier ("kubernetes").
func (p *ComponentProvider) GetType() string {
	defer perf.Track(nil, "kubernetes.GetType")()
	return "kubernetes"
}

// GetGroup returns the human-readable group label used in command help ("Kubernetes").
func (p *ComponentProvider) GetGroup() string {
	defer perf.Track(nil, "kubernetes.GetGroup")()
	return "Kubernetes"
}

// GetBasePath returns the configured base path for Kubernetes components, or the
// default when unset.
func (p *ComponentProvider) GetBasePath(atmosConfig *schema.AtmosConfiguration) string {
	defer perf.Track(atmosConfig, "kubernetes.GetBasePath")()

	if atmosConfig == nil || atmosConfig.Components.Kubernetes.BasePath == "" {
		return DefaultConfig().BasePath
	}
	return atmosConfig.Components.Kubernetes.BasePath
}

// ListComponents returns the sorted names of Kubernetes components defined in the stack config.
func (p *ComponentProvider) ListComponents(ctx context.Context, stack string, stackConfig map[string]any) ([]string, error) {
	defer perf.Track(nil, "kubernetes.ListComponents")()

	componentsSection, ok := stackConfig["components"].(map[string]any)
	if !ok {
		return []string{}, nil
	}

	kubernetesComponents, ok := componentsSection["kubernetes"].(map[string]any)
	if !ok {
		return []string{}, nil
	}

	componentNames := make([]string, 0, len(kubernetesComponents))
	for name := range kubernetesComponents {
		componentNames = append(componentNames, name)
	}
	sort.Strings(componentNames)
	return componentNames, nil
}

// ValidateComponent validates a Kubernetes component config. Abstract components are
// skipped. The provider, when present, must be a string of "kubectl" or "kustomize";
// a non-string provider is rejected so the type error surfaces at validation time.
func (p *ComponentProvider) ValidateComponent(config map[string]any) error {
	defer perf.Track(nil, "kubernetes.ValidateComponent")()

	if config == nil {
		return nil
	}

	if metadata, ok := config["metadata"].(map[string]any); ok {
		if componentType, ok := metadata["type"].(string); ok && componentType == "abstract" {
			return nil
		}
	}

	if rawProvider, exists := config["provider"]; exists {
		provider, ok := rawProvider.(string)
		if !ok {
			return fmt.Errorf("%w: %w", errUtils.ErrComponentValidationFailed, errUtils.ErrKubernetesProviderType)
		}
		switch provider {
		case "", ProviderKubectl, ProviderKustomize:
		default:
			return fmt.Errorf("%w: provider must be %q or %q", errUtils.ErrComponentValidationFailed, ProviderKubectl, ProviderKustomize)
		}
	}

	return nil
}

// Execute dispatches the requested subcommand to the corresponding Kubernetes operation.
func (p *ComponentProvider) Execute(ctx *component.ExecutionContext) error {
	defer perf.Track(ctx.AtmosConfig, "kubernetes.Execute")()

	switch ctx.SubCommand {
	case "render":
		return executeOperation(ctx, OperationRender)
	case "diff", "plan":
		return executeOperation(ctx, OperationDiff)
	case "apply", "deploy":
		return executeOperation(ctx, OperationApply)
	case "delete":
		return executeOperation(ctx, OperationDelete)
	case "validate":
		return executeOperation(ctx, OperationValidate)
	default:
		return fmt.Errorf("%w: %q", errUtils.ErrKubernetesUnsupportedSubcommand, ctx.SubCommand)
	}
}

// GenerateArtifacts is a no-op for Kubernetes components; manifests are produced at execution time.
func (p *ComponentProvider) GenerateArtifacts(ctx *component.ExecutionContext) error {
	defer perf.Track(nil, "kubernetes.GenerateArtifacts")()
	return nil
}

// GetAvailableCommands returns the subcommands supported by the Kubernetes component type.
func (p *ComponentProvider) GetAvailableCommands() []string {
	defer perf.Track(nil, "kubernetes.GetAvailableCommands")()
	return []string{"render", "diff", "plan", "apply", "deploy", "delete", "validate"}
}
