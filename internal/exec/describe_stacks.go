package exec

import (
	"errors"
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
	"strings"
)

// ExecuteDescribeStacks executes `describe stacks` command
func ExecuteDescribeStacks(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()

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

	var configAndStacksInfo c.ConfigAndStacksInfo
	stacksMap, err := FindStacksMap(configAndStacksInfo, false)
	if err != nil {
		return err
	}

	finalStacksMap := map[string]interface{}{}

	for stackName, stack := range stacksMap {
		delete(stack.(map[interface{}]interface{}), "imports")

		if len(components) > 0 {
			if componentsSection, ok := stack.(map[interface{}]interface{})["components"].(map[string]interface{}); ok {
				if terraformSection, ok2 := componentsSection["terraform"].(map[string]interface{}); ok2 {
					for comp, _ := range terraformSection {
						if u.SliceContainsString(components, comp) {
							finalStacksMap[stackName] = stack
						}
					}
				}
				if helmfileSection, ok3 := componentsSection["helmfile"].(map[string]interface{}); ok3 {
					for comp, _ := range helmfileSection {
						if u.SliceContainsString(components, comp) {
							finalStacksMap[stackName] = stack
						}
					}
				}
			}
		} else {
			finalStacksMap[stackName] = stack
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
