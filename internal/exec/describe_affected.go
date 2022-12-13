package exec

import (
	"fmt"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteDescribeAffectedCmd executes `describe affected` command
func ExecuteDescribeAffectedCmd(cmd *cobra.Command, args []string) error {
	info, err := processCommandLineArgs("", cmd, args)
	if err != nil {
		return err
	}

	cliConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		u.PrintErrorToStdError(err)
		return err
	}

	flags := cmd.Flags()

	format, err := flags.GetString("format")
	if err != nil {
		return err
	}
	if format != "" && format != "yaml" && format != "json" {
		return fmt.Errorf("invalid '--format' flag '%s'. Valid values are 'yaml' (default) and 'json'", format)
	}
	if format == "" {
		format = "json"
	}

	file, err := flags.GetString("file")
	if err != nil {
		return err
	}

	stacksMap, err := FindStacksMap(cliConfig)
	if err != nil {
		return err
	}

	finalStacksMap := make(map[string]any)
	var varsSection map[any]any
	var stackName string

	for stackFileName, stackSection := range stacksMap {
		// Delete the stack-wide imports
		delete(stackSection.(map[any]any), "imports")

		if componentsSection, ok := stackSection.(map[any]any)["components"].(map[string]any); ok {

			if terraformSection, ok := componentsSection["terraform"].(map[string]any); ok {
				for componentName, compSection := range terraformSection {
					componentSection, ok := compSection.(map[string]any)
					if !ok {
						return fmt.Errorf("invalid 'components.terraform.%s' section in the file '%s'", componentName, stackFileName)
					}

					// Component vars
					if varsSection, ok = componentSection["vars"].(map[any]any); ok {
						context := cfg.GetContextFromVars(varsSection)
						stackName, err = cfg.GetContextPrefix(stackFileName, context, cliConfig.Stacks.NamePattern, stackFileName)
						if err != nil {
							return err
						}
					}

					if stackName == "" {
						stackName = stackFileName
					}

					if !u.MapKeyExists(finalStacksMap, stackName) {
						finalStacksMap[stackName] = make(map[string]any)
					}

					if !u.MapKeyExists(finalStacksMap[stackName].(map[string]any), "components") {
						finalStacksMap[stackName].(map[string]any)["components"] = make(map[string]any)
					}
					if !u.MapKeyExists(finalStacksMap[stackName].(map[string]any)["components"].(map[string]any), "terraform") {
						finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["terraform"] = make(map[string]any)
					}
					if !u.MapKeyExists(finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any), componentName) {
						finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)[componentName] = make(map[string]any)
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
