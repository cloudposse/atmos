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
	defer perf.Track(nil, "component.NewResolver")()

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
		err := errUtils.Build(errUtils.ErrComponentTypeMismatch).
			WithHintf("Path resolves to `%s` component but command expects `%s` component\nYou ran: `atmos %s <command> %s`\nThe path points to: `%s` component",
				componentInfo.ComponentType, expectedComponentType, expectedComponentType, path, componentInfo.ComponentType).
			WithHint("Run the correct command for this component type").
			WithContext("path", path).
			WithContext("resolved_type", componentInfo.ComponentType).
			WithContext("expected_type", expectedComponentType).
			WithContext("component", componentInfo.FullComponent).
			WithExitCode(2).
			Err()
		return "", err
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

// ResolveComponentFromPathWithoutValidation resolves a filesystem path to a component name without stack validation.
//
// This function extracts the component name from a filesystem path without validating it exists in a stack.
// It's used during command-line argument parsing to convert path-based component arguments (like ".")
// into component names. Stack validation happens later in ProcessStacks() to avoid duplicate work.
//
// Parameters:
//   - atmosConfig: Atmos configuration
//   - path: Filesystem path (can be ".", relative, or absolute)
//   - expectedComponentType: Expected component type ("terraform", "helmfile", "packer")
//
// Returns:
//   - Component name extracted from path (e.g., "vpc/security-group" from "/path/to/components/terraform/vpc/security-group")
//   - Error if path cannot be resolved or component type doesn't match
func (r *Resolver) ResolveComponentFromPathWithoutValidation(
	atmosConfig *schema.AtmosConfiguration,
	path string,
	expectedComponentType string,
) (string, error) {
	defer perf.Track(atmosConfig, "component.ResolveComponentFromPathWithoutValidation")()

	log.Debug("Resolving component from path (without validation)",
		"path", path,
		"expected_type", expectedComponentType,
	)

	// 1. Extract component info from path.
	componentInfo, err := u.ExtractComponentInfoFromPath(atmosConfig, path)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrPathResolutionFailed, err)
	}

	// 2. Verify component type matches.
	if componentInfo.ComponentType != expectedComponentType {
		err := errUtils.Build(errUtils.ErrComponentTypeMismatch).
			WithHintf("Path resolves to `%s` component but command expects `%s` component\nYou ran: `atmos %s <command> %s`\nThe path points to: `%s` component",
				componentInfo.ComponentType, expectedComponentType, expectedComponentType, path, componentInfo.ComponentType).
			WithHint("Run the correct command for this component type").
			WithContext("path", path).
			WithContext("resolved_type", componentInfo.ComponentType).
			WithContext("expected_type", expectedComponentType).
			WithContext("component", componentInfo.FullComponent).
			WithExitCode(2).
			Err()
		return "", err
	}

	log.Debug("Successfully resolved component from path (without validation)",
		"path", path,
		"component", componentInfo.FullComponent,
		"type", componentInfo.ComponentType,
	)

	return componentInfo.FullComponent, nil
}

// validateComponentInStack checks if a component exists in the specified stack configuration.
// Returns the actual stack key (which may be an alias) that matched the component.

// LoadStackConfig loads and validates that a stack exists.
func (r *Resolver) loadStackConfig(
	atmosConfig *schema.AtmosConfiguration,
	stack string,
	componentName string,
) (map[string]any, error) {
	// Load all stacks using the injected stack loader.
	stacksMap, _, err := r.stackLoader.FindStacksMap(atmosConfig, false)
	if err != nil {
		loadErr := errUtils.Build(errUtils.ErrStackNotFound).
			WithHintf("Failed to load stack configurations: %s\n\nPath-based component resolution requires valid stack configuration", err.Error()).
			WithHint("Run `atmos describe config` to see stack configuration paths\nVerify your stack manifests are in the configured stacks directory").
			WithContext("stack", stack).
			WithContext("component", componentName).
			WithContext("underlying_error", err.Error()).
			WithExitCode(2).
			Err()
		return nil, loadErr
	}

	// Check if stack exists.
	stackConfig, ok := stacksMap[stack]
	if !ok {
		notFoundErr := errUtils.Build(errUtils.ErrStackNotFound).
			WithHintf("Stack `%s` not found", stack).
			WithHint("Run `atmos list stacks` to see all available stacks\nVerify the stack name matches your stack manifest files").
			WithContext("stack", stack).
			WithContext("component", componentName).
			WithExitCode(2).
			Err()
		return nil, notFoundErr
	}

	// Validate stack configuration format.
	stackConfigMap, ok := stackConfig.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid stack configuration for '%s'", stack)
	}

	return stackConfigMap, nil
}

// extractComponentsSection extracts the component type section from a stack configuration.
func extractComponentsSection(
	stackConfigMap map[string]any,
	componentType string,
	stack string,
) (map[string]any, error) {
	// Extract components section.
	components, ok := stackConfigMap["components"]
	if !ok {
		return nil, fmt.Errorf("%w: stack '%s' has no components section", errUtils.ErrComponentNotInStack, stack)
	}

	componentsMap, ok := components.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: invalid components section in stack '%s'", errUtils.ErrComponentNotInStack, stack)
	}

	// Extract component type section (terraform/helmfile/packer).
	typeComponents, ok := componentsMap[componentType]
	if !ok {
		return nil, fmt.Errorf(
			"%w: stack '%s' has no %s components",
			errUtils.ErrComponentNotInStack,
			stack,
			componentType,
		)
	}

	typeComponentsMap, ok := typeComponents.(map[string]any)
	if !ok {
		return nil, fmt.Errorf(
			"%w: invalid %s components section in stack '%s'",
			errUtils.ErrComponentNotInStack,
			componentType,
			stack,
		)
	}

	return typeComponentsMap, nil
}

// findComponentMatches searches for a component by name, checking both direct keys and aliases.
// Returns all matching stack keys to handle ambiguous cases.
func findComponentMatches(
	typeComponentsMap map[string]any,
	componentName string,
) []string {
	// First check for direct key match.
	if _, exists := typeComponentsMap[componentName]; exists {
		return []string{componentName}
	}

	// If no direct match, check for aliases via component or metadata.component fields.
	// Collect ALL matches to detect ambiguous cases.
	var matches []string
	for stackKey, componentConfig := range typeComponentsMap {
		componentConfigMap, ok := componentConfig.(map[string]any)
		if !ok {
			continue
		}

		// Check 'component' field.
		if comp, ok := componentConfigMap["component"].(string); ok && comp == componentName {
			matches = append(matches, stackKey)
			continue
		}

		// Check 'metadata.component' field.
		if metadata, ok := componentConfigMap["metadata"].(map[string]any); ok {
			if metaComp, ok := metadata["component"].(string); ok && metaComp == componentName {
				matches = append(matches, stackKey)
			}
		}
	}

	return matches
}

// handleComponentMatches processes the match results and returns appropriate errors or the resolved component.
func handleComponentMatches(
	matches []string,
	componentName string,
	stack string,
	componentType string,
) (string, error) {
	if len(matches) == 0 {
		err := errUtils.Build(errUtils.ErrComponentNotInStack).
			WithHintf("Component `%s` not found in stack `%s`", componentName, stack).
			WithHintf("Run `atmos list stacks --component %s` to see stacks containing this component\nRun `atmos list components --stack %s` to see components in this stack",
				componentName, stack).
			WithContext("component", componentName).
			WithContext("stack", stack).
			WithContext("component_type", componentType).
			WithExitCode(2).
			Err()
		return "", err
	}

	if len(matches) > 1 {
		matchesStr := ""
		for i, match := range matches {
			if i > 0 {
				matchesStr += ", "
			}
			matchesStr += match
		}
		err := errUtils.Build(errUtils.ErrAmbiguousComponentPath).
			WithHintf("Path resolves to `%s` which is referenced by multiple components in stack `%s`\nMatching components: %s",
				componentName, stack, matchesStr).
			WithHintf("Use the exact component name instead of a path\nExample: `atmos %s <command> %s --stack %s`",
				componentType, matches[0], stack).
			WithContext("path_component", componentName).
			WithContext("stack", stack).
			WithContext("matches", matchesStr).
			WithContext("match_count", fmt.Sprintf("%d", len(matches))).
			WithExitCode(2).
			Err()
		return "", err
	}

	// Exactly one match found.
	return matches[0], nil
}

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

	// Load and validate stack configuration.
	stackConfigMap, err := r.loadStackConfig(atmosConfig, stack, componentName)
	if err != nil {
		return "", err
	}

	// Extract the component type section.
	typeComponentsMap, err := extractComponentsSection(stackConfigMap, componentType, stack)
	if err != nil {
		return "", err
	}

	// Find all matching components.
	matches := findComponentMatches(typeComponentsMap, componentName)

	// Handle match results (none, one, or multiple).
	resolvedComponent, err := handleComponentMatches(matches, componentName, stack, componentType)
	if err != nil {
		return "", err
	}

	// Log success.
	if resolvedComponent == componentName {
		log.Debug("Component validated successfully in stack (direct match)",
			"component", componentName,
			"stack", stack,
			"type", componentType,
		)
	} else {
		log.Debug("Component validated successfully in stack (alias match)",
			"path_component", componentName,
			"stack_key", resolvedComponent,
			"stack", stack,
			"type", componentType,
		)
	}

	return resolvedComponent, nil
}
