package exec

import (
	c "github.com/cloudposse/atmos/pkg/config"
	s "github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
	"strings"
)

// ExecuteTerraformGenerateVarfilesCmd executes `terraform generate varfiles` command
func ExecuteTerraformGenerateVarfilesCmd(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()

	fileTemplate, err := flags.GetString("file-template")
	if err != nil {
		return err
	}

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

	return ExecuteTerraformGenerateVarfiles(fileTemplate, stacks, components)
}

// ExecuteTerraformGenerateVarfiles generates varfiles for all terraform components in all stacks
func ExecuteTerraformGenerateVarfiles(fileTemplate string, stacks []string, components []string) error {
	var configAndStacksInfo c.ConfigAndStacksInfo
	stacksMap, err := FindStacksMap(configAndStacksInfo, false)
	if err != nil {
		return err
	}

	for stackName, stackSection := range stacksMap {
		if len(stacks) == 0 || u.SliceContainsString(stacks, stackName) {
			if componentsSection, ok := stackSection.(map[any]any)["components"].(map[string]any); ok {
				if terraformSection, ok := componentsSection["terraform"].(map[string]any); ok {
					for componentName, compSection := range terraformSection {
						if componentSection, ok := compSection.(map[string]any); ok {
							// Find all derived components of the provided components
							derivedComponents, err := s.FindComponentsDerivedFromBaseComponents(stackName, terraformSection, components)
							if err != nil {
								return err
							}

							if len(components) == 0 || u.SliceContainsString(components, componentName) || u.SliceContainsString(derivedComponents, componentName) {
								if varsSection, ok := componentSection["vars"].(map[any]any); ok {
									context := c.GetContextFromVars(varsSection)

									if context.Component == "" {

									}
								}

								terraformComponent := componentName
								if componentAttribute, ok := componentSection["component"].(string); ok {
									terraformComponent = componentAttribute
								}

								u.PrintInfo(terraformComponent)
							}
						}
					}
				}
			}
		}
	}

	return nil
}
