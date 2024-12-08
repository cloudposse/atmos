package list

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/samber/lo"

	"github.com/cloudposse/atmos/pkg/schema"
)

// getStackComponents extracts Terraform components from the final map of stacks
func getStackComponents(stackData any, listConfig schema.ListConfig) ([]string, error) {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("could not parse stacks")
	}

	componentsMap, ok := stackMap["components"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("could not parse components")
	}

	terraformComponents, ok := componentsMap["terraform"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("could not parse Terraform components")
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	header := make([]string, 0)
	values := make([]string, 0)

	re := regexp.MustCompile(`\{\{\s*(.*?)\s*\}\}`)

	for _, v := range listConfig.Columns {
		header = append(header, v.Name)
		match := re.FindStringSubmatch(v.Value)

		if len(match) > 1 {
			values = append(values, match[1])
		} else {
			return nil, fmt.Errorf("invalid value format for column name %s", v.Name)
		}
	}
	fmt.Fprintln(writer, strings.Join(header, "\t\t"))

	uniqueKeys := lo.Keys(terraformComponents)

	for _, dataKey := range uniqueKeys {
		data := terraformComponents[dataKey]
		result := make([]string, 0)
		for _, key := range values {
			value, found := resolveKey(data.(map[string]any), key)
			if !found {
				value = "-"
			}
			result = append(result, fmt.Sprintf("%s", value))
		}
		fmt.Fprintln(writer, strings.Join(result, "\t\t"))
	}
	writer.Flush()
	return lo.Keys(terraformComponents), nil
}

// resolveKey resolves a key from a map, supporting nested keys with dot notation
func resolveKey(data map[string]any, key string) (any, bool) {
	// Remove leading dot from the key (e.g., `.vars.tenant` -> `vars.tenant`)
	key = strings.TrimPrefix(key, ".")

	// Split key on `.`
	parts := strings.Split(key, ".")
	current := data

	// Traverse the map for each part
	for i, part := range parts {
		if i == len(parts)-1 {
			// Return the value for the last part
			if value, exists := current[part]; exists {
				return value, true
			}
			return nil, false
		}

		// Traverse deeper
		if nestedMap, ok := current[part].(map[string]any); ok {
			current = nestedMap
		} else {
			return nil, false
		}
	}

	return nil, false
}

// FilterAndListComponents filters and lists components based on the given stack
func FilterAndListComponents(stackFlag string, stacksMap map[string]any, listConfig schema.ListConfig) (string, error) {
	components := []string{}

	if stackFlag != "" {
		// Filter components for the specified stack
		if stackData, ok := stacksMap[stackFlag]; ok {
			stackComponents, err := getStackComponents(stackData, listConfig)
			if err != nil {
				return "", fmt.Errorf("error processing stack '%s': %w", stackFlag, err)
			}
			components = append(components, stackComponents...)
		} else {
			return "", fmt.Errorf("stack '%s' not found", stackFlag)
		}
	} else {
		// Get all components from all stacks
		for _, stackData := range stacksMap {
			stackComponents, err := getStackComponents(stackData, listConfig)
			if err != nil {
				continue // Skip invalid stacks
			}
			components = append(components, stackComponents...)
		}
	}

	// Remove duplicates and sort components
	components = lo.Uniq(components)
	sort.Strings(components)

	if len(components) == 0 {
		return "No components found", nil
	}
	return strings.Join(components, "\n") + "\n", nil
}
