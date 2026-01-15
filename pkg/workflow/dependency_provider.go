package workflow

import (
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// DefaultDependencyProvider implements DependencyProvider using the real dependencies package.
type DefaultDependencyProvider struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewDefaultDependencyProvider creates a new DefaultDependencyProvider.
func NewDefaultDependencyProvider(atmosConfig *schema.AtmosConfiguration) *DefaultDependencyProvider {
	defer perf.Track(atmosConfig, "workflow.NewDefaultDependencyProvider")()

	return &DefaultDependencyProvider{
		atmosConfig: atmosConfig,
	}
}

// LoadToolVersionsDependencies loads tools from .tool-versions file.
func (p *DefaultDependencyProvider) LoadToolVersionsDependencies() (map[string]string, error) {
	defer perf.Track(p.atmosConfig, "workflow.DefaultDependencyProvider.LoadToolVersionsDependencies")()

	return dependencies.LoadToolVersionsDependencies(p.atmosConfig)
}

// ResolveWorkflowDependencies extracts tool dependencies from a workflow definition.
func (p *DefaultDependencyProvider) ResolveWorkflowDependencies(workflowDef *schema.WorkflowDefinition) (map[string]string, error) {
	defer perf.Track(p.atmosConfig, "workflow.DefaultDependencyProvider.ResolveWorkflowDependencies")()

	resolver := dependencies.NewResolver(p.atmosConfig)
	return resolver.ResolveWorkflowDependencies(workflowDef)
}

// MergeDependencies merges two dependency maps, with overlay taking precedence.
func (p *DefaultDependencyProvider) MergeDependencies(base, overlay map[string]string) (map[string]string, error) {
	defer perf.Track(p.atmosConfig, "workflow.DefaultDependencyProvider.MergeDependencies")()

	return dependencies.MergeDependencies(base, overlay)
}

// EnsureTools ensures all required tools are installed.
func (p *DefaultDependencyProvider) EnsureTools(deps map[string]string) error {
	defer perf.Track(p.atmosConfig, "workflow.DefaultDependencyProvider.EnsureTools")()

	installer := dependencies.NewInstaller(p.atmosConfig)
	return installer.EnsureTools(deps)
}

// UpdatePathForTools updates PATH to include tool binaries.
func (p *DefaultDependencyProvider) UpdatePathForTools(deps map[string]string) error {
	defer perf.Track(p.atmosConfig, "workflow.DefaultDependencyProvider.UpdatePathForTools")()

	return dependencies.UpdatePathForTools(p.atmosConfig, deps)
}
