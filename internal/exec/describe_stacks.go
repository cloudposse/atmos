package exec

import (
	"errors"
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	s "github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
	"strings"
)

// ExecuteDescribeStacks executes `describe stacks` command
func ExecuteDescribeStacks(cmd *cobra.Command, args []string) error {
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
		return errors.New(fmt.Sprintf("Invalid '--format' flag '%s'. Valid values are 'yaml' (default) and 'json'", format))
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

	var configAndStacksInfo c.ConfigAndStacksInfo
	configAndStacksInfo.Stack = filterByStack
	stacksMap, err := FindStacksMap(configAndStacksInfo, filterByStack != "")
	if err != nil {
		return err
	}

	finalStacksMap := make(map[string]interface{})

	for stackName, stackSection := range stacksMap {
		if filterByStack == "" || filterByStack == stackName {
			// Delete the stack-wide imports
			delete(stackSection.(map[interface{}]interface{}), "imports")

			if !u.MapKeyExists(finalStacksMap, stackName) {
				finalStacksMap[stackName] = make(map[string]interface{})
			}

			if componentsSection, ok := stackSection.(map[interface{}]interface{})["components"].(map[string]interface{}); ok {
				if len(componentTypes) == 0 || u.SliceContainsString(componentTypes, "terraform") {
					if terraformSection, ok := componentsSection["terraform"].(map[string]interface{}); ok {
						for componentName, compSection := range terraformSection {
							componentSection, ok := compSection.(map[string]interface{})
							if !ok {
								return errors.New(fmt.Sprintf("Invalid 'components.terraform.%s' section in the file '%s'", componentName, stackName))
							}

							// Find all derived components of the provided components and include them in the output
							derivedComponents, err := s.FindComponentsDerivedFromBaseComponents(stackName, terraformSection, components)
							if err != nil {
								return err
							}

							if len(components) == 0 || u.SliceContainsString(components, componentName) || u.SliceContainsString(derivedComponents, componentName) {
								if !u.MapKeyExists(finalStacksMap[stackName].(map[string]interface{}), "components") {
									finalStacksMap[stackName].(map[string]interface{})["components"] = make(map[string]interface{})
								}
								if !u.MapKeyExists(finalStacksMap[stackName].(map[string]interface{})["components"].(map[string]interface{}), "terraform") {
									finalStacksMap[stackName].(map[string]interface{})["components"].(map[string]interface{})["terraform"] = make(map[string]interface{})
								}
								if !u.MapKeyExists(finalStacksMap[stackName].(map[string]interface{})["components"].(map[string]interface{})["terraform"].(map[string]interface{}), componentName) {
									finalStacksMap[stackName].(map[string]interface{})["components"].(map[string]interface{})["terraform"].(map[string]interface{})[componentName] = make(map[string]interface{})
								}

								for sectionName, section := range componentSection {
									if len(sections) == 0 || u.SliceContainsString(sections, sectionName) {
										finalStacksMap[stackName].(map[string]interface{})["components"].(map[string]interface{})["terraform"].(map[string]interface{})[componentName].(map[string]interface{})[sectionName] = section
									}
								}
							}
						}
					}
				}

				if len(componentTypes) == 0 || u.SliceContainsString(componentTypes, "helmfile") {
					if helmfileSection, ok := componentsSection["helmfile"].(map[string]interface{}); ok {
						for componentName, compSection := range helmfileSection {
							componentSection, ok := compSection.(map[string]interface{})
							if !ok {
								return errors.New(fmt.Sprintf("Invalid 'components.helmfile.%s' section in the file '%s'", componentName, stackName))
							}

							// Find all derived components of the provided components and include them in the output
							derivedComponents, err := s.FindComponentsDerivedFromBaseComponents(stackName, helmfileSection, components)
							if err != nil {
								return err
							}

							if len(components) == 0 || u.SliceContainsString(components, componentName) || u.SliceContainsString(derivedComponents, componentName) {
								if !u.MapKeyExists(finalStacksMap[stackName].(map[string]interface{}), "components") {
									finalStacksMap[stackName].(map[string]interface{})["components"] = make(map[string]interface{})
								}
								if !u.MapKeyExists(finalStacksMap[stackName].(map[string]interface{})["components"].(map[string]interface{}), "helmfile") {
									finalStacksMap[stackName].(map[string]interface{})["components"].(map[string]interface{})["helmfile"] = make(map[string]interface{})
								}
								if !u.MapKeyExists(finalStacksMap[stackName].(map[string]interface{})["components"].(map[string]interface{})["helmfile"].(map[string]interface{}), componentName) {
									finalStacksMap[stackName].(map[string]interface{})["components"].(map[string]interface{})["helmfile"].(map[string]interface{})[componentName] = make(map[string]interface{})
								}

								for sectionName, section := range componentSection {
									if len(sections) == 0 || u.SliceContainsString(sections, sectionName) {
										finalStacksMap[stackName].(map[string]interface{})["components"].(map[string]interface{})["helmfile"].(map[string]interface{})[componentName].(map[string]interface{})[sectionName] = section
									}
								}
							}
						}
					}
				}
			}

			// Filter out empty stacks (stacks without any components)
			if st, ok := finalStacksMap[stackName].(map[string]interface{}); ok {
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
