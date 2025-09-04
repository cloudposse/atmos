package list

import (
	"errors"
	"fmt"
	"sort"

	"github.com/samber/lo"
)

// Error definitions for component listing.
var (
	// ErrParseStacks is returned when stack data cannot be parsed.
	ErrParseStacks = errors.New("could not parse stacks")
	// ErrParseComponents is returned when component data cannot be parsed.
	ErrParseComponents = errors.New("could not parse components")
	// ErrStackNotFound is returned when a requested stack is not found.
	ErrStackNotFound = errors.New("stack not found")
	// ErrProcessStack is returned when there's an error processing a stack.
	ErrProcessStack = errors.New("error processing stack")
)

// getStackComponents extracts components from the final map of stacks.
func getStackComponents(stackData any) ([]string, error) {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return nil, ErrParseStacks
	}

	componentsMap, ok := stackMap["components"].(map[string]any)
	if !ok {
		return nil, ErrParseComponents
	}

	// Initialize empty maps for each component type
	terraformComponents := make(map[string]any)
	helmfileComponents := make(map[string]any)
	packerComponents := make(map[string]any)
	ansibleComponents := make(map[string]any)

	// Only try to get components if they exist, no error if they don't
	if comp, ok := componentsMap["terraform"].(map[string]any); ok {
		terraformComponents = comp
	}

	if comp, ok := componentsMap["helmfile"].(map[string]any); ok {
		helmfileComponents = comp
	}

	if comp, ok := componentsMap["packer"].(map[string]any); ok {
		packerComponents = comp
	}

	if comp, ok := componentsMap["ansible"].(map[string]any); ok {
		ansibleComponents = comp
	}

	// Merge all component maps into one.
	allComponents := lo.Assign(terraformComponents, helmfileComponents, packerComponents, ansibleComponents)

	return lo.Keys(allComponents), nil
}

// getComponentsForSpecificStack extracts components from a specific stack.
func getComponentsForSpecificStack(stackName string, stacksMap map[string]any) ([]string, error) {
	// Verify stack exists.
	stackData, ok := stacksMap[stackName]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrStackNotFound, stackName)
	}

	// Get components for the specific stack.
	stackComponents, err := getStackComponents(stackData)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrProcessStack, stackName, err)
	}

	return stackComponents, nil
}

// processAllStacks collects components from all valid stacks.
func processAllStacks(stacksMap map[string]any) []string {
	var components []string
	for _, stackData := range stacksMap {
		stackComponents, err := getStackComponents(stackData)
		if err != nil {
			continue // Skip invalid stacks.
		}
		components = append(components, stackComponents...)
	}
	return components
}

// FilterAndListComponents filters and lists components based on the given stack.
func FilterAndListComponents(stackFlag string, stacksMap map[string]any) ([]string, error) {
	var components []string
	if stacksMap == nil {
		return nil, fmt.Errorf("%w: %s", ErrStackNotFound, stackFlag)
	}

	// Handle specific stack case.
	if stackFlag != "" {
		stackComponents, err := getComponentsForSpecificStack(stackFlag, stacksMap)
		if err != nil {
			return nil, err
		}
		components = stackComponents
	} else {
		// Process all stacks.
		components = processAllStacks(stacksMap)
	}

	// Remove duplicates and sort components.
	components = lo.Uniq(components)
	sort.Strings(components)

	if len(components) == 0 {
		return []string{}, nil
	}
	return components, nil
}
