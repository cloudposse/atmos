package list

import (
	"sort"

	"github.com/samber/lo"
)

// FilterAndListStacks filters stacks by the given component. The component is
// matched under any component type section (terraform, aws/cloudformation,
// helmfile, packer, container, ansible, kubernetes, helm, etc) -- this function is
// shared by completion/prompt callers across component types (e.g.
// cmd/container/completions.go, cmd/ansible/completions.go), so it must not assume
// terraform is the only component type or it silently returns zero matches for
// every other type.
func FilterAndListStacks(stacksMap map[string]any, component string) ([]string, error) {
	if component != "" {
		// Filter stacks by component
		filteredStacks := []string{}
		for stackName, stackData := range stacksMap {
			if stackContainsAnyTypeComponent(stackData, component) {
				filteredStacks = append(filteredStacks, stackName)
			}
		}

		if len(filteredStacks) == 0 {
			return nil, nil
		}
		sort.Strings(filteredStacks)
		return filteredStacks, nil
	}

	// List all stacks
	stacks := lo.Keys(stacksMap)
	sort.Strings(stacks)
	return stacks, nil
}

// stackContainsAnyTypeComponent reports whether stackData defines component under
// any component type section.
func stackContainsAnyTypeComponent(stackData any, component string) bool {
	v2, ok := stackData.(map[string]any)
	if !ok {
		return false
	}
	components, ok := v2["components"].(map[string]any)
	if !ok {
		return false
	}
	for _, typeSection := range components {
		typeMap, ok := typeSection.(map[string]any)
		if !ok {
			continue
		}
		if _, exists := typeMap[component]; exists {
			return true
		}
	}
	return false
}
