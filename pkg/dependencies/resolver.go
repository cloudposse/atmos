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
// Merges 3 scopes from stack configuration (lowest to highest priority):
//  1. Global scope: top-level dependencies
//  2. Component type scope: terraform.dependencies / helmfile.dependencies / packer.dependencies
//  3. Component instance scope: components.terraform.vpc.dependencies
//
// Parameters:
//   - componentType: "terraform", "helmfile", or "packer"
//   - stackConfig: Full merged stack configuration (includes all scopes)
//   - componentConfig: Merged component configuration
func (r *Resolver) ResolveComponentDependencies(
	componentType string,
	stackConfig map[string]any,
	componentConfig map[string]any,
) (map[string]string, error) {
	defer perf.Track(r.atmosConfig, "dependencies.ResolveComponentDependencies")()

	// Start with empty dependencies.
	merged := make(map[string]string)

	// Scope 1: Global dependencies (top-level dependencies in stack config).
	if globalDeps := extractDependenciesFromConfig(stackConfig); globalDeps != nil {
		var err error
		merged, err = MergeDependencies(merged, globalDeps)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to merge global dependencies: %w", errUtils.ErrDependencyResolution, err)
		}
	}

	// Scope 2: Component type dependencies (terraform.dependencies in stack config).
	if componentType != "" {
		var err error
		merged, err = mergeComponentTypeDeps(merged, stackConfig, componentType)
		if err != nil {
			return nil, err
		}
	}

	// Scope 3: Component instance dependencies (components.terraform.vpc.dependencies).
	if componentDeps := extractDependenciesFromConfig(componentConfig); componentDeps != nil {
		var err error
		merged, err = MergeDependencies(merged, componentDeps)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to merge component instance dependencies: %w", errUtils.ErrDependencyResolution, err)
		}
	}

	return merged, nil
}

// mergeComponentTypeDeps merges component type dependencies from the stack config.
func mergeComponentTypeDeps(merged map[string]string, stackConfig map[string]any, componentType string) (map[string]string, error) {
	typeConfig, hasType := stackConfig[componentType].(map[string]any)
	if !hasType {
		return merged, nil
	}

	typeDeps := extractDependenciesFromConfig(typeConfig)
	if typeDeps == nil {
		return merged, nil
	}

	result, err := MergeDependencies(merged, typeDeps)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to merge %s component type dependencies: %w", errUtils.ErrDependencyResolution, componentType, err)
	}
	return result, nil
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
