package dependencies

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Resolver resolves tool dependencies with inheritance and validation.
type Resolver struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewResolver creates a new dependency resolver.
func NewResolver(atmosConfig *schema.AtmosConfiguration) *Resolver {
	defer perf.Track(nil, "dependencies.NewResolver")()

	return &Resolver{
		atmosConfig: atmosConfig,
	}
}

// ResolveWorkflowDependencies resolves tool dependencies for a workflow.
func (r *Resolver) ResolveWorkflowDependencies(workflowDef *schema.WorkflowDefinition) (map[string]string, error) {
	defer perf.Track(r.atmosConfig, "dependencies.ResolveWorkflowDependencies")()

	if workflowDef == nil {
		return map[string]string{}, nil
	}

	if workflowDef.Dependencies == nil || workflowDef.Dependencies.Tools == nil {
		return map[string]string{}, nil
	}

	return workflowDef.Dependencies.Tools, nil
}

// ResolveCommandDependencies resolves tool dependencies for a custom command.
func (r *Resolver) ResolveCommandDependencies(command *schema.Command) (map[string]string, error) {
	defer perf.Track(r.atmosConfig, "dependencies.ResolveCommandDependencies")()

	if command == nil {
		return map[string]string{}, nil
	}

	if command.Dependencies == nil || command.Dependencies.Tools == nil {
		return map[string]string{}, nil
	}

	return command.Dependencies.Tools, nil
}

// ResolveComponentDependencies resolves tool dependencies for a component.
// Merges: stack catalog → stack instance → component catalog → component instance.
func (r *Resolver) ResolveComponentDependencies(
	componentConfig map[string]any,
	stackConfig map[string]any,
) (map[string]string, error) {
	defer perf.Track(r.atmosConfig, "dependencies.ResolveComponentDependencies")()

	// Start with empty dependencies
	merged := make(map[string]string)

	// 1. Get stack-level dependencies (if any)
	if stackDeps := extractDependenciesFromConfig(stackConfig); stackDeps != nil {
		var err error
		merged, err = MergeDependencies(merged, stackDeps)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to merge stack dependencies: %w", errUtils.ErrDependencyResolution, err)
		}
	}

	// 2. Get component-level dependencies (if any)
	if componentDeps := extractDependenciesFromConfig(componentConfig); componentDeps != nil {
		var err error
		merged, err = MergeDependencies(merged, componentDeps)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to merge component dependencies: %w", errUtils.ErrDependencyResolution, err)
		}
	}

	return merged, nil
}

// extractDependenciesFromConfig extracts dependencies.tools from a config map.
func extractDependenciesFromConfig(config map[string]any) map[string]string {
	if config == nil {
		return nil
	}

	// Look for dependencies.tools
	depsInterface, hasDeps := config["dependencies"]
	if !hasDeps {
		return nil
	}

	depsMap, ok := depsInterface.(map[string]any)
	if !ok {
		return nil
	}

	toolsInterface, hasTools := depsMap["tools"]
	if !hasTools {
		return nil
	}

	toolsMap, ok := toolsInterface.(map[string]any)
	if !ok {
		return nil
	}

	// Convert map[string]any to map[string]string
	tools := make(map[string]string)
	for tool, versionInterface := range toolsMap {
		if version, ok := versionInterface.(string); ok {
			tools[tool] = version
		}
	}

	return tools
}
