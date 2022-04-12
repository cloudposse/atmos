package exec

import (
	"errors"
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
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

	component, err := flags.GetString("component")
	if err != nil {
		return err
	}

	var configAndStacksInfo c.ConfigAndStacksInfo
	stacksMap, err := FindStacksMap(configAndStacksInfo, false)
	if err != nil {
		return err
	}

	finalStacksMap := map[string]interface{}{}

	if component != "" {
		for stackName, stack := range stacksMap {
			delete(stack.(map[interface{}]interface{}), "imports")
			finalStacksMap[stackName] = stack
		}
		//for stackName := range stacksMap {
		//if stackSection, ok := stacksMap[stackName].(map[interface{}]interface{}); !ok {
		//	return nil, nil, nil, nil, "", "", "", nil, false, nil, errors.New(fmt.Sprintf("Could not find the stack '%s'", stack))
		//}
		//if componentsSection, ok = stackSection["components"].(map[string]interface{}); !ok {
		//	return nil, nil, nil, nil, "", "", "", nil, false, nil, errors.New(fmt.Sprintf("'components' section is missing in the stack '%s'", stack))
		//}
		//if componentTypeSection, ok = componentsSection[componentType].(map[string]interface{}); !ok {
		//	return nil, nil, nil, nil, "", "", "", nil, false, nil, errors.New(fmt.Sprintf("'components/%s' section is missing in the stack '%s'", componentType, stack))
		//}
		//if componentSection, ok = componentTypeSection[component].(map[string]interface{}); !ok {
		//	return nil, nil, nil, nil, "", "", "", nil, false, nil, errors.New(fmt.Sprintf("Invalid or missing configuration for the component '%s' in the stack '%s'", component, stack))
		//}
		//}
	} else {
		for stackName, stack := range stacksMap {
			delete(stack.(map[interface{}]interface{}), "imports")
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
