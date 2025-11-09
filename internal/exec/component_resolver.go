package exec

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

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
//
// Example:
//
//	componentName, err := ResolveComponentFromPath(cfg, ".", "dev", "terraform")
//	// Returns: "vpc/security-group" if current dir is components/terraform/vpc/security-group
func ResolveComponentFromPath(
	atmosConfig *schema.AtmosConfiguration,
	path string,
	stack string,
	expectedComponentType string,
) (string, error) {
	defer perf.Track(atmosConfig, "exec.ResolveComponentFromPath")()

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

	// 3. If stack is specified, validate component exists in stack.
	if stack != "" {
		if err := validateComponentInStack(atmosConfig, componentInfo.FullComponent, stack, expectedComponentType); err != nil {
			return "", err
		}
	}

	log.Debug("Successfully resolved component from path",
		"path", path,
		"component", componentInfo.FullComponent,
		"type", componentInfo.ComponentType,
		"stack", stack,
	)

	return componentInfo.FullComponent, nil
}

// ResolveComponentFromPathWithoutTypeCheck resolves a filesystem path to a component name without validating component type.
//
// This function is used by describe component which auto-detects component type,
// so we only need to:
// 1. Extract component name from the filesystem path
// 2. Validate the component exists in the specified stack configuration (if stack is provided)
//
// The component type is NOT validated - caller is responsible for type detection/validation.
//
// Parameters:
//   - atmosConfig: Atmos configuration
//   - path: Filesystem path (can be ".", relative, or absolute)
//   - stack: Stack name to validate against (empty string to skip stack validation)
//
// Returns:
//   - Component name as it appears in stack configuration (e.g., "vpc/security-group")
//   - Error if path cannot be resolved or component doesn't exist in stack
//
// Example:
//
//	componentName, err := ResolveComponentFromPathWithoutTypeCheck(cfg, ".", "dev")
//	// Returns: "vpc/security-group" if current dir is components/terraform/vpc/security-group
func ResolveComponentFromPathWithoutTypeCheck(
	atmosConfig *schema.AtmosConfiguration,
	path string,
	stack string,
) (string, error) {
	defer perf.Track(atmosConfig, "exec.ResolveComponentFromPathWithoutTypeCheck")()

	log.Debug("Resolving component from path (without type check)",
		"path", path,
		"stack", stack,
	)

	// 1. Extract component info from path.
	componentInfo, err := u.ExtractComponentInfoFromPath(atmosConfig, path)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrPathResolutionFailed, err)
	}

	// 2. If stack is specified, validate component exists in stack (using detected component type).
	if stack != "" {
		if err := validateComponentInStack(atmosConfig, componentInfo.FullComponent, stack, componentInfo.ComponentType); err != nil {
			return "", err
		}
	}

	log.Debug("Successfully resolved component from path (without type check)",
		"path", path,
		"component", componentInfo.FullComponent,
		"detected_type", componentInfo.ComponentType,
		"stack", stack,
	)

	return componentInfo.FullComponent, nil
}

// validateComponentInStack checks if a component exists in the specified stack configuration.
func validateComponentInStack(
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	stack string,
	componentType string,
) error {
	defer perf.Track(atmosConfig, "exec.validateComponentInStack")()

	log.Debug("Validating component exists in stack",
		"component", componentName,
		"stack", stack,
		"type", componentType,
	)

	// Load all stacks.
	stacksMap, _, err := FindStacksMap(atmosConfig, false)
	if err != nil {
		return fmt.Errorf("failed to load stacks: %w", err)
	}

	// Check if stack exists.
	stackConfig, ok := stacksMap[stack]
	if !ok {
		return fmt.Errorf("stack '%s' not found", stack)
	}

	// Extract components section.
	stackConfigMap, ok := stackConfig.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid stack configuration for '%s'", stack)
	}

	components, ok := stackConfigMap["components"]
	if !ok {
		return fmt.Errorf("%w: stack '%s' has no components section", errUtils.ErrComponentNotInStack, stack)
	}

	componentsMap, ok := components.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: invalid components section in stack '%s'", errUtils.ErrComponentNotInStack, stack)
	}

	// Extract component type section (terraform/helmfile/packer).
	typeComponents, ok := componentsMap[componentType]
	if !ok {
		return fmt.Errorf(
			"%w: stack '%s' has no %s components",
			errUtils.ErrComponentNotInStack,
			stack,
			componentType,
		)
	}

	typeComponentsMap, ok := typeComponents.(map[string]any)
	if !ok {
		return fmt.Errorf(
			"%w: invalid %s components section in stack '%s'",
			errUtils.ErrComponentNotInStack,
			componentType,
			stack,
		)
	}

	// Check if component exists.
	if _, exists := typeComponentsMap[componentName]; !exists {
		return fmt.Errorf(
			"%w: component '%s' not found in stack '%s' (type: %s)",
			errUtils.ErrComponentNotInStack,
			componentName,
			stack,
			componentType,
		)
	}

	log.Debug("Component validated successfully in stack",
		"component", componentName,
		"stack", stack,
		"type", componentType,
	)

	return nil
}
