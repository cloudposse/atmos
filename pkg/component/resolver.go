package component

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// StackLoader is an interface for loading stack configurations.
// This allows the component package to avoid depending on internal/exec.
type StackLoader interface {
	FindStacksMap(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) (
		map[string]any,
		map[string]map[string]any,
		error,
	)
}

// Resolver handles component path resolution and validation.
type Resolver struct {
	stackLoader StackLoader
}

// NewResolver creates a new component resolver with the given stack loader.
func NewResolver(stackLoader StackLoader) *Resolver {
	return &Resolver{stackLoader: stackLoader}
}

// ResolveComponentFromPath resolves a filesystem path to a component name and validates it exists in the stack.
//
// This function:
// 1. Extracts component type and name from the filesystem path
// 2. Verifies the component type matches the expected type
// 3. Validates the component exists in the specified stack configuration (if stack is provided)
//
// Parameters:
//   - atmosConfig: Atmos configuration
//   - path: Filesystem path (can be ".", relative, or absolute)
//   - stack: Stack name to validate against (empty string to skip stack validation)
//   - expectedComponentType: Expected component type ("terraform", "helmfile", "packer")
//
// Returns:
//   - Component name as it appears in stack configuration (e.g., "vpc/security-group")
//   - Error if path cannot be resolved or component doesn't exist in stack
func (r *Resolver) ResolveComponentFromPath(
	atmosConfig *schema.AtmosConfiguration,
	path string,
	stack string,
	expectedComponentType string,
) (string, error) {
	defer perf.Track(atmosConfig, "component.ResolveComponentFromPath")()

	log.Debug("Resolving component from path",
		"path", path,
		"stack", stack,
		"expected_type", expectedComponentType,
	)

	// 1. Extract component info from path.
	componentInfo, err := u.ExtractComponentInfoFromPath(atmosConfig, path)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrPathResolutionFailed, err)
	}

	// 2. Verify component type matches.
	if componentInfo.ComponentType != expectedComponentType {
		return "", fmt.Errorf(
			"%w: path resolves to %s component but command expects %s component",
			errUtils.ErrComponentTypeMismatch,
			componentInfo.ComponentType,
			expectedComponentType,
		)
	}

	// 3. If stack is specified, validate component exists in stack and get actual stack key.
	var resolvedComponent string
	if stack != "" {
		stackKey, err := r.validateComponentInStack(atmosConfig, componentInfo.FullComponent, stack, expectedComponentType)
		if err != nil {
			return "", err
		}
		resolvedComponent = stackKey
	} else {
		resolvedComponent = componentInfo.FullComponent
	}

	log.Debug("Successfully resolved component from path",
		"path", path,
		"component", resolvedComponent,
		"type", componentInfo.ComponentType,
		"stack", stack,
	)

	return resolvedComponent, nil
}

// ResolveComponentFromPathWithoutTypeCheck resolves a filesystem path to a component name without validating component type.
func (r *Resolver) ResolveComponentFromPathWithoutTypeCheck(
	atmosConfig *schema.AtmosConfiguration,
	path string,
	stack string,
) (string, error) {
	defer perf.Track(atmosConfig, "component.ResolveComponentFromPathWithoutTypeCheck")()

	log.Debug("Resolving component from path (without type check)",
		"path", path,
		"stack", stack,
	)

	// 1. Extract component info from path.
	componentInfo, err := u.ExtractComponentInfoFromPath(atmosConfig, path)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrPathResolutionFailed, err)
	}

	// 2. If stack is specified, validate component exists in stack and get actual stack key.
	var resolvedComponent string
	if stack != "" {
		stackKey, err := r.validateComponentInStack(atmosConfig, componentInfo.FullComponent, stack, componentInfo.ComponentType)
		if err != nil {
			return "", err
		}
		resolvedComponent = stackKey
	} else {
		resolvedComponent = componentInfo.FullComponent
	}

	log.Debug("Successfully resolved component from path (without type check)",
		"path", path,
		"component", resolvedComponent,
		"detected_type", componentInfo.ComponentType,
		"stack", stack,
	)

	return resolvedComponent, nil
}

// validateComponentInStack checks if a component exists in the specified stack configuration.
// Returns the actual stack key (which may be an alias) that matched the component.
func (r *Resolver) validateComponentInStack(
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	stack string,
	componentType string,
) (string, error) {
	defer perf.Track(atmosConfig, "component.validateComponentInStack")()

	log.Debug("Validating component exists in stack",
		"component", componentName,
		"stack", stack,
		"type", componentType,
	)

	// Load all stacks using the injected stack loader.
	stacksMap, _, err := r.stackLoader.FindStacksMap(atmosConfig, false)
	if err != nil {
		return "", fmt.Errorf("failed to load stacks: %w", err)
	}

	// Check if stack exists.
	stackConfig, ok := stacksMap[stack]
	if !ok {
		return "", fmt.Errorf("stack '%s' not found", stack)
	}

	// Extract components section.
	stackConfigMap, ok := stackConfig.(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid stack configuration for '%s'", stack)
	}

	components, ok := stackConfigMap["components"]
	if !ok {
		return "", fmt.Errorf("%w: stack '%s' has no components section", errUtils.ErrComponentNotInStack, stack)
	}

	componentsMap, ok := components.(map[string]any)
	if !ok {
		return "", fmt.Errorf("%w: invalid components section in stack '%s'", errUtils.ErrComponentNotInStack, stack)
	}

	// Extract component type section (terraform/helmfile/packer).
	typeComponents, ok := componentsMap[componentType]
	if !ok {
		return "", fmt.Errorf(
			"%w: stack '%s' has no %s components",
			errUtils.ErrComponentNotInStack,
			stack,
			componentType,
		)
	}

	typeComponentsMap, ok := typeComponents.(map[string]any)
	if !ok {
		return "", fmt.Errorf(
			"%w: invalid %s components section in stack '%s'",
			errUtils.ErrComponentNotInStack,
			componentType,
			stack,
		)
	}

	// First check for direct key match.
	if _, exists := typeComponentsMap[componentName]; exists {
		log.Debug("Component validated successfully in stack (direct match)",
			"component", componentName,
			"stack", stack,
			"type", componentType,
		)
		return componentName, nil
	}

	// If no direct match, check for aliases via component or metadata.component fields.
	for stackKey, componentConfig := range typeComponentsMap {
		componentConfigMap, ok := componentConfig.(map[string]any)
		if !ok {
			continue
		}

		// Check 'component' field.
		if comp, ok := componentConfigMap["component"].(string); ok && comp == componentName {
			log.Debug("Component validated successfully in stack (alias via component field)",
				"path_component", componentName,
				"stack_key", stackKey,
				"stack", stack,
				"type", componentType,
			)
			return stackKey, nil
		}

		// Check 'metadata.component' field.
		if metadata, ok := componentConfigMap["metadata"].(map[string]any); ok {
			if metaComp, ok := metadata["component"].(string); ok && metaComp == componentName {
				log.Debug("Component validated successfully in stack (alias via metadata.component field)",
					"path_component", componentName,
					"stack_key", stackKey,
					"stack", stack,
					"type", componentType,
				)
				return stackKey, nil
			}
		}
	}

	return "", fmt.Errorf(
		"%w: component '%s' not found in stack '%s' (type: %s)",
		errUtils.ErrComponentNotInStack,
		componentName,
		stack,
		componentType,
	)
}
