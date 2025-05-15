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
	// ErrParseTerraformComponents is returned when terraform component data cannot be parsed.
	ErrParseTerraformComponents = errors.New("could not parse Terraform components")
	// ErrStackNotFound is returned when a requested stack is not found.
	ErrStackNotFound = errors.New("stack not found")
	// ErrProcessStack is returned when there's an error processing a stack.
	ErrProcessStack = errors.New("error processing stack")
)

// getStackComponents extracts Terraform components from the final map of stacks.
func getStackComponents(stackData any) ([]string, error) {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return nil, ErrParseStacks
	}

	componentsMap, ok := stackMap["components"].(map[string]any)
	if !ok {
		return nil, ErrParseComponents
	}

	terraformComponents, ok := componentsMap["terraform"].(map[string]any)
	if !ok {
		return nil, ErrParseTerraformComponents
	}

	return lo.Keys(terraformComponents), nil
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

// GetComponentsForDriftDetection returns a list of components that have drift detection enabled
func GetComponentsForDriftDetection(stacksMap map[string]any) ([]string, error) {
	if stacksMap == nil {
		return nil, fmt.Errorf("stacks map is nil")
	}

	var components []string

	// Iterate through all stacks
	for _, stackData := range stacksMap {
		stackDataMap, ok := stackData.(map[string]any)
		if !ok {
			continue
		}

		// Get components section
		componentsSection, ok := stackDataMap["components"].(map[string]any)
		if !ok {
			continue
		}

		// Get terraform components
		terraformComponents, ok := componentsSection["terraform"].(map[string]any)
		if !ok {
			continue
		}

		// Check each component for drift detection settings
		// We only support drift detection for pro, terraform components at this time
		for componentName, componentData := range terraformComponents {
			componentDataMap, ok := componentData.(map[string]any)
			if !ok {
				continue
			}

			settings, ok := componentDataMap["settings"].(map[string]any)
			if !ok {
				continue
			}

			pro, ok := settings["pro"].(map[string]any)
			if !ok {
				continue
			}

			proEnabled, ok := pro["enabled"].(bool)
			if !ok || !proEnabled {
				continue
			}

			// Check drift detection settings
			driftDetection, ok := pro["drift_detection"].(map[string]any)
			if !ok {
				continue
			}

			// Check if drift detection is enabled
			driftEnabled, ok := driftDetection["enabled"].(bool)
			if !ok || !driftEnabled {
				continue
			}

			components = append(components, componentName)
		}
	}

	// Remove duplicates and sort
	components = lo.Uniq(components)
	sort.Strings(components)

	return components, nil
}

// UploadDriftDetection uploads components with drift detection enabled to the pro API
func UploadDriftDetection(components []string) error {
	// TODO: Implement actual API call
	// For now, just print the components that would be uploaded
	fmt.Printf("Would upload the following components for drift detection:\n")
	for _, component := range components {
		fmt.Printf("- %s\n", component)
	}
	return nil
}
