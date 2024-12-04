package list

import (
	"fmt"
	"sort"
	"strings"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/fatih/color"
	"github.com/samber/lo"
)

// getStackComponents extracts Terraform components from the final map of stacks
func getStackComponents(stackData any) ([]string, error) {
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

	return lo.Keys(terraformComponents), nil
}

// FilterAndListComponents filters and lists components based on the given stack
func FilterAndListComponents(stackFlag string) (string, error) {
	components := []string{}

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	cliConfig, err := config.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		u.PrintMessageInColor(fmt.Sprintf("Error initializing CLI config: %v", err), color.New(color.FgRed))
	}

	stacksMap, err := e.ExecuteDescribeStacks(cliConfig, "", nil, nil, nil, false, false, false)
	if err != nil {
		u.PrintMessageInColor(fmt.Sprintf("Error describing stacks: %v", err), color.New(color.FgRed))
	}

	if stackFlag != "" {
		// Filter components for the specified stack
		if stackData, ok := stacksMap[stackFlag]; ok {
			stackComponents, err := getStackComponents(stackData)
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
			stackComponents, err := getStackComponents(stackData)
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
