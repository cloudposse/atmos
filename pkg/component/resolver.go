package component

import (
	"errors"
	"fmt"
	"sort"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	// ComponentKey is the key used for component fields in stack configuration maps.
	ComponentKey = "component"
	// TypeKey is the key used for component type fields in stack configuration maps.
	TypeKey = "type"
	// StackKey is the key used for stack fields in logging and context.
	StackKey = "stack"
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
		// Return the error directly to preserve detailed hints and exit codes.
		// ExtractComponentInfoFromPath already wraps errors appropriately.
		return "", err
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
		// Return the error directly to preserve detailed hints and exit codes.
		// ExtractComponentInfoFromPath already wraps errors appropriately.
		return "", err
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
		ComponentKey, resolvedComponent,
		"detected_type", componentInfo.ComponentType,
		StackKey, stack,
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
		// Return the error directly to preserve detailed hints and exit codes.
		// ExtractComponentInfoFromPath already wraps errors appropriately.
		return "", err
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
			WithCause(err).
			WithHintf("Failed to load stack configurations: %s\n\nPath-based component resolution requires valid stack configuration", err.Error()).
			WithHint("Run `atmos describe config` to see stack configuration paths\nVerify your stack manifests are in the configured stacks directory").
			WithContext("stack", stack).
			WithContext("component", componentName).
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
		return nil, fmt.Errorf("%w for '%s'", errUtils.ErrInvalidStackConfiguration, stack)
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
// This function finds ALL components that reference the given Terraform component path,
// including both direct key matches and components that reference it via metadata.component.
func findComponentMatches(
	typeComponentsMap map[string]any,
	componentName string,
) []string {
	var matches []string

	// Iterate through all components to find matches.
	// We need to check all components, not just direct key matches, because:
	// 1. A direct key match (e.g., "vpc") means the Atmos component name equals the Terraform folder
	// 2. An alias match means a different Atmos component references the same Terraform folder
	// Both cases should be collected to detect ambiguous paths.
	for stackKey, componentConfig := range typeComponentsMap {
		// Check for direct key match first.
		if stackKey == componentName {
			matches = append(matches, stackKey)
			continue
		}

		componentConfigMap, ok := componentConfig.(map[string]any)
		if !ok {
			continue
		}

		// Check 'component' field for alias.
		if comp, ok := componentConfigMap[ComponentKey].(string); ok && comp == componentName {
			matches = append(matches, stackKey)
			continue
		}

		// Check 'metadata.component' field for alias.
		if metadata, ok := componentConfigMap["metadata"].(map[string]any); ok {
			if metaComp, ok := metadata[ComponentKey].(string); ok && metaComp == componentName {
				matches = append(matches, stackKey)
			}
		}
	}

	// Sort matches for deterministic ordering.
	// This ensures consistent error messages and selector order.
	sort.Strings(matches)

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
		return handleNoMatches(componentName, stack, componentType)
	}

	if len(matches) > 1 {
		return handleMultipleMatches(matches, componentName, stack, componentType)
	}

	// Exactly one match found.
	return matches[0], nil
}

// handleNoMatches returns an error when no component matches are found.
func handleNoMatches(componentName, stack, componentType string) (string, error) {
	err := errUtils.Build(errUtils.ErrComponentNotInStack).
		WithHintf("Component `%s` not found in stack `%s`", componentName, stack).
		WithHintf("Run `atmos list stacks --component %s` to see stacks containing this component\nRun `atmos list components --stack %s` to see components in this stack",
			componentName, stack).
		WithContext("component", componentName).
		WithContext(StackKey, stack).
		WithContext("component_type", componentType).
		WithExitCode(2).
		Err()
	return "", err
}

// handleMultipleMatches handles the case when multiple components match.
// In interactive terminals, prompts user to select. Otherwise returns an error.
func handleMultipleMatches(matches []string, componentName, stack, componentType string) (string, error) {
	// Check if we're in an interactive terminal.
	term := terminal.New()
	if !term.IsTTY(terminal.Stderr) {
		// Non-interactive terminal - return error.
		return "", buildAmbiguousComponentError(matches, componentName, stack, componentType)
	}

	// Interactive terminal - prompt user to select.
	log.Debug("Multiple component matches found, prompting user for selection",
		"matches", matches,
		"component", componentName,
		StackKey, stack,
	)

	selected, err := promptForComponentSelection(matches, componentName, stack)
	if err != nil {
		// If user aborted, return specific error.
		if errors.Is(err, errUtils.ErrUserAborted) {
			return "", errUtils.ErrUserAborted
		}
		// Other error - fall back to ambiguous path error.
		log.Debug("Component selection failed, falling back to error",
			"error", err.Error(),
		)
		return "", buildAmbiguousComponentError(matches, componentName, stack, componentType)
	}

	// User made a selection - return it.
	log.Info("User selected component from interactive prompt",
		"selected", selected,
		"matches", matches,
		StackKey, stack,
	)
	return selected, nil
}

// buildAmbiguousComponentError builds an error for ambiguous component paths.
func buildAmbiguousComponentError(matches []string, componentName, stack, componentType string) error {
	matchesStr := ""
	for i, match := range matches {
		if i > 0 {
			matchesStr += ", "
		}
		matchesStr += match
	}

	// Use first match as example, guaranteed to exist since len(matches) > 1.
	exampleComponent := matches[0]

	return errUtils.Build(errUtils.ErrAmbiguousComponentPath).
		WithHintf("Path resolves to `%s` which is referenced by multiple components in stack `%s`\nMatching components: %s",
			componentName, stack, matchesStr).
		WithHintf("Use the exact component name instead of a path\nExample: `atmos %s <command> %s --stack %s`",
			componentType, exampleComponent, stack).
		WithContext("path_component", componentName).
		WithContext(StackKey, stack).
		WithContext("matches", matchesStr).
		WithContext("match_count", fmt.Sprintf("%d", len(matches))).
		WithExitCode(2).
		Err()
}

// promptForComponentSelection prompts the user to select from multiple matching components.
// Uses Charmbracelet's Huh library for interactive selection.
// Returns the selected component name or an error if user aborts or selection fails.
func promptForComponentSelection(
	matches []string,
	componentName string,
	stack string,
) (string, error) {
	var selected string

	// Build options - just show component names for simplicity.
	// Future enhancement: could show vars or metadata for each option.
	options := make([]huh.Option[string], len(matches))
	for i, match := range matches {
		options[i] = huh.NewOption(match, match)
	}

	// Configure keymap for abort.
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "cancel"),
	)

	// Create selector with Atmos theme.
	selector := huh.NewSelect[string]().
		Title(fmt.Sprintf("Component path '%s' matches multiple instances in stack '%s'", componentName, stack)).
		Description("Select which component instance to use (ctrl+c to cancel)").
		Options(options...).
		Value(&selected).
		WithTheme(uiutils.NewAtmosHuhTheme()).
		WithKeyMap(keyMap)

	// Run interactive prompt.
	if err := selector.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", errUtils.ErrUserAborted
		}
		return "", fmt.Errorf("component selection failed: %w", err)
	}

	log.Debug("User selected component",
		"selected", selected,
		"from_matches", matches,
		StackKey, stack,
	)

	return selected, nil
}

func (r *Resolver) validateComponentInStack(
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	stack string,
	componentType string,
) (string, error) {
	defer perf.Track(atmosConfig, "component.validateComponentInStack")()

	log.Debug("Validating component exists in stack",
		ComponentKey, componentName,
		StackKey, stack,
		TypeKey, componentType,
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
			ComponentKey, componentName,
			StackKey, stack,
			TypeKey, componentType,
		)
	} else {
		log.Debug("Component validated successfully in stack (alias match)",
			"path_component", componentName,
			"stack_key", resolvedComponent,
			StackKey, stack,
			TypeKey, componentType,
		)
	}

	return resolvedComponent, nil
}
