package exec

import "github.com/cloudposse/atmos/pkg/filematch"

// GetFilesToBeDeleted retrieves file paths to be deleted based on stack and component filters.
func GetFilesToBeDeleted(stackMap map[string]any, component, stack string) ([]string, error) {
	var paths []string
	for stackName, stackInfo := range stackMap {
		if !isValidStack(stackInfo, stackName, stack) {
			continue
		}
		newPaths, err := processStackInfo(stackInfo, component)
		if err != nil {
			return nil, err
		}
		paths = append(paths, newPaths...)
	}
	return paths, nil
}

// isValidStack checks if the stack info is valid and matches the stack filter.
func isValidStack(stackInfo any, stackName, stack string) bool {
	if stackInfo == nil {
		return false
	}
	if stack != "" && stackName != stack {
		return false
	}
	return true
}

// processStackInfo processes stack information to extract file paths.
func processStackInfo(stackInfo any, component string) ([]string, error) {
	info, ok := stackInfo.(map[string]any)
	if !ok {
		return nil, nil
	}
	components, ok := info["components"].(map[string]any)
	if !ok {
		return nil, nil
	}
	return processComponents(components, component)
}

// processComponents processes component types to extract file paths.
func processComponents(components map[string]any, component string) ([]string, error) {
	var paths []string
	for componentType, componentTypeMap := range components {
		if componentType != "terraform" {
			continue
		}
		newPaths, err := processComponentType(componentTypeMap, component)
		if err != nil {
			return nil, err
		}
		paths = append(paths, newPaths...)
	}
	return paths, nil
}

// processComponentType processes a component type map to extract file paths.
func processComponentType(componentTypeMap any, component string) ([]string, error) {
	typeMap, ok := componentTypeMap.(map[string]any)
	if !ok {
		return nil, nil
	}
	var paths []string
	for componentName, componentValue := range typeMap {
		if component != "" && componentName != component {
			continue
		}
		newPaths, err := extractCleanPatterns(componentValue)
		if err != nil {
			return nil, err
		}
		paths = append(paths, newPaths...)
	}
	return paths, nil
}

// extractCleanPatterns extracts and matches clean patterns from component settings.
func extractCleanPatterns(componentValue any) ([]string, error) {
	componentMap, ok := componentValue.(map[string]any)
	if !ok {
		return nil, nil
	}
	settings, ok := componentMap["settings"].(map[string]any)
	if !ok {
		return nil, nil
	}
	cleanPatterns, ok := settings["clean"].([]any)
	if !ok {
		return nil, nil
	}
	return filematch.NewGlobMatcher().MatchFiles(convertToStringArray(cleanPatterns))
}

// convertToStringArray converts an array of any to an array of strings.
func convertToStringArray(arr []any) []string {
	result := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
}
