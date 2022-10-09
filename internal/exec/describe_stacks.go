package exec

import (
	"fmt"
	"strings"

	c "github.com/cloudposse/atmos/pkg/config"
	s "github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// ExecuteDescribeStacks executes `describe stacks` command
func ExecuteDescribeStacks(cmd *cobra.Command, args []string) error {
	cliConfig, err := c.InitCliConfig(c.ConfigAndStacksInfo{}, true)
	if err != nil {
		u.PrintErrorToStdError(err)
		return err
	}

	flags := cmd.Flags()

	filterByStack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	format, err := flags.GetString("format")
	if err != nil {
		return err
	}
	if format != "" && format != "yaml" && format != "json" {
		return fmt.Errorf("invalid '--format' flag '%s'. Valid values are 'yaml' (default) and 'json'", format)
	}
	if format == "" {
		format = "yaml"
	}

	file, err := flags.GetString("file")
	if err != nil {
		return err
	}

	componentsCsv, err := flags.GetString("components")
	if err != nil {
		return err
	}
	var components []string
	if componentsCsv != "" {
		components = strings.Split(componentsCsv, ",")
	}

	componentTypesCsv, err := flags.GetString("component-types")
	if err != nil {
		return err
	}
	var componentTypes []string
	if componentTypesCsv != "" {
		componentTypes = strings.Split(componentTypesCsv, ",")
	}

	sectionsCsv, err := flags.GetString("sections")
	if err != nil {
		return err
	}
	var sections []string
	if sectionsCsv != "" {
		sections = strings.Split(sectionsCsv, ",")
	}

	stacksMap, err := FindStacksMap(cliConfig)
	if err != nil {
		return err
	}

	finalStacksMap := make(map[string]any)

	for stackName, stackSection := range stacksMap {
		if filterByStack == "" || filterByStack == stackName {
			// Delete the stack-wide imports
			delete(stackSection.(map[any]any), "imports")

			if !u.MapKeyExists(finalStacksMap, stackName) {
				finalStacksMap[stackName] = make(map[string]any)
			}

			if componentsSection, ok := stackSection.(map[any]any)["components"].(map[string]any); ok {
				if len(componentTypes) == 0 || u.SliceContainsString(componentTypes, "terraform") {
					if terraformSection, ok := componentsSection["terraform"].(map[string]any); ok {
						for componentName, compSection := range terraformSection {
							componentSection, ok := compSection.(map[string]any)
							if !ok {
								return fmt.Errorf("invalid 'components.terraform.%s' section in the file '%s'", componentName, stackName)
							}

							// Find all derived components of the provided components and include them in the output
							derivedComponents, err := s.FindComponentsDerivedFromBaseComponents(stackName, terraformSection, components)
							if err != nil {
								return err
							}

							if len(components) == 0 || u.SliceContainsString(components, componentName) || u.SliceContainsString(derivedComponents, componentName) {
								if !u.MapKeyExists(finalStacksMap[stackName].(map[string]any), "components") {
									finalStacksMap[stackName].(map[string]any)["components"] = make(map[string]any)
								}
								if !u.MapKeyExists(finalStacksMap[stackName].(map[string]any)["components"].(map[string]any), "terraform") {
									finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["terraform"] = make(map[string]any)
								}
								if !u.MapKeyExists(finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any), componentName) {
									finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)[componentName] = make(map[string]any)
								}

								for sectionName, section := range componentSection {
									if len(sections) == 0 || u.SliceContainsString(sections, sectionName) {
										finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)[componentName].(map[string]any)[sectionName] = section
									}
								}
							}
						}
					}
				}

				if len(componentTypes) == 0 || u.SliceContainsString(componentTypes, "helmfile") {
					if helmfileSection, ok := componentsSection["helmfile"].(map[string]any); ok {
						for componentName, compSection := range helmfileSection {
							componentSection, ok := compSection.(map[string]any)
							if !ok {
								return fmt.Errorf("invalid 'components.helmfile.%s' section in the file '%s'", componentName, stackName)
							}

							// Find all derived components of the provided components and include them in the output
							derivedComponents, err := s.FindComponentsDerivedFromBaseComponents(stackName, helmfileSection, components)
							if err != nil {
								return err
							}

							if len(components) == 0 || u.SliceContainsString(components, componentName) || u.SliceContainsString(derivedComponents, componentName) {
								if !u.MapKeyExists(finalStacksMap[stackName].(map[string]any), "components") {
									finalStacksMap[stackName].(map[string]any)["components"] = make(map[string]any)
								}
								if !u.MapKeyExists(finalStacksMap[stackName].(map[string]any)["components"].(map[string]any), "helmfile") {
									finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["helmfile"] = make(map[string]any)
								}
								if !u.MapKeyExists(finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["helmfile"].(map[string]any), componentName) {
									finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["helmfile"].(map[string]any)[componentName] = make(map[string]any)
								}

								for sectionName, section := range componentSection {
									if len(sections) == 0 || u.SliceContainsString(sections, sectionName) {
										finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["helmfile"].(map[string]any)[componentName].(map[string]any)[sectionName] = section
									}
								}
							}
						}
					}
				}
			}

			// Filter out empty stacks (stacks without any components)
			if st, ok := finalStacksMap[stackName].(map[string]any); ok {
				if len(st) == 0 {
					delete(finalStacksMap, stackName)
				}
			}
		}
	}

	if format == "yaml" {
		if file == "" {
			err = u.PrintAsYAML(finalStacksMap)
			if err != nil {
				return err
			}
		} else {
			err = u.WriteToFileAsYAML(file, finalStacksMap, 0644)
			if err != nil {
				return err
			}
		}
	} else if format == "json" {
		if file == "" {
			err = u.PrintAsJSON(finalStacksMap)
			if err != nil {
				return err
			}
		} else {
			err = u.WriteToFileAsJSON(file, finalStacksMap, 0644)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
