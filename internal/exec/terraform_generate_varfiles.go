package exec

import (
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	s "github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
	"strings"
)

// ExecuteTerraformGenerateVarfilesCmd executes `terraform generate varfiles` command
func ExecuteTerraformGenerateVarfilesCmd(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()

	stacksCsv, err := flags.GetString("stacks")
	if err != nil {
		return err
	}
	var stacks []string
	if stacksCsv != "" {
		stacks = strings.Split(stacksCsv, ",")
	}

	componentsCsv, err := flags.GetString("components")
	if err != nil {
		return err
	}
	var components []string
	if componentsCsv != "" {
		components = strings.Split(componentsCsv, ",")
	}

	return ExecuteTerraformGenerateVarfiles(stacks, components)
}

// ExecuteTerraformGenerateVarfiles generates varfiles for all terraform components in all stacks
func ExecuteTerraformGenerateVarfiles(stacks []string, components []string) error {
	var configAndStacksInfo c.ConfigAndStacksInfo
	stacksMap, err := FindStacksMap(configAndStacksInfo, false)
	if err != nil {
		return err
	}

	for stackName, stackSection := range stacksMap {
		if len(stacks) == 0 || u.SliceContainsString(stacks, stackName) {
			u.PrintInfo(fmt.Sprintf("Processing stack '%s'", stackName))

			if componentsSection, ok := stackSection.(map[any]any)["components"].(map[string]any); ok {
				if terraformSection, ok := componentsSection["terraform"].(map[string]any); ok {
					for componentName, compSection := range terraformSection {
						componentSection, ok := compSection.(map[string]any)
						if !ok {
							return fmt.Errorf("invalid 'components.terraform.%s' section in the file '%s'", componentName, stackName)
						}

						// Find all derived components of the provided components
						derivedComponents, err := s.FindComponentsDerivedFromBaseComponents(stackName, terraformSection, components)
						if err != nil {
							return err
						}

						if len(components) == 0 || u.SliceContainsString(components, componentName) || u.SliceContainsString(derivedComponents, componentName) {
							if _, ok := componentSection["vars"].(map[any]any); ok {
								u.PrintInfo(fmt.Sprintf("Processing component '%s'", componentName))
							}
						}
					}
				}
			}
		}
	}

	return nil
}
