package exec

import (
	c "atmos/internal/config"
	s "atmos/internal/stack"
	u "atmos/internal/utils"
	"fmt"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"path"
)

// ExecuteTerraform executes terraform commands
func ExecuteTerraform(cmd *cobra.Command, args []string) error {
	if len(args) < 3 {
		return errors.New("Invalid number of arguments")
	}

	cmd.DisableFlagParsing = false

	err := cmd.ParseFlags(args)
	if err != nil {
		return err
	}
	flags := cmd.Flags()

	// Get stack
	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	// Process and merge CLI configurations
	err = c.InitConfig(stack)
	if err != nil {
		return err
	}

	// Process CLI arguments and flags
	additionalArgsAndFlags := removeCommonArgsAndFlags(args)
	subCommand := args[0]
	allArgsAndFlags := append([]string{subCommand}, additionalArgsAndFlags...)

	// Process stack config file(s)
	_, stacksMap, err := s.ProcessYAMLConfigFiles(
		c.Config.StacksBaseAbsolutePath,
		c.Config.StackConfigFiles,
		false,
		true)

	if err != nil {
		return err
	}

	// Check if component was provided
	componentFromArg := args[1]
	if len(componentFromArg) < 1 {
		return errors.New("'component' is required")
	}

	// Check and process stacks
	var stackSection map[interface{}]interface{}
	var componentsSection map[string]interface{}
	var terraformSection map[string]interface{}
	var componentSection map[string]interface{}
	var componentVarsSection map[interface{}]interface{}
	var baseComponent string
	var ok bool

	if c.Config.StackType == "Directory" {
		if stackSection, ok = stacksMap[stack].(map[interface{}]interface{}); !ok {
			return errors.New(fmt.Sprintf("Stack '%s' does not exist in %s", stack, c.Config.StackConfigFiles[0]))
		}
		if componentsSection, ok = stackSection["components"].(map[string]interface{}); !ok {
			return errors.New(fmt.Sprintf("'components' section is missing in stack '%s'", stack))
		}
		if terraformSection, ok = componentsSection["terraform"].(map[string]interface{}); !ok {
			return errors.New(fmt.Sprintf("'components/terraform' section is missing in stack '%s'", stack))
		}
		if componentSection, ok = terraformSection[componentFromArg].(map[string]interface{}); !ok {
			return errors.New(fmt.Sprintf("Invalid or missing configuration for component '%s' in stack '%s'", componentFromArg, stack))
		}
		if componentVarsSection, ok = componentSection["vars"].(map[interface{}]interface{}); !ok {
			return errors.New(fmt.Sprintf("Missing 'vars' section for component '%s' in stack '%s'", componentFromArg, stack))
		}
		if baseComponent, ok = componentSection["component"].(string); !ok {
			baseComponent = ""
		}

		color.Cyan("Variables for component '%s' in stack '%s':", componentFromArg, stack)
		err = u.PrintAsYAML(componentVarsSection)
		if err != nil {
			return err
		}
	} else {
		//for k,v := range stacksMap {
		//	if stack == k {
		//		if i, ok := v["vars"]; ok {
		//			globalVarsSection = i.(map[interface{}]interface{})
		//		}
		//	}
		//}
	}

	// Check component (and its base component)
	component := componentFromArg
	if len(baseComponent) > 0 {
		component = baseComponent
	}

	componentPath := path.Join(c.Config.TerraformDirAbsolutePath, component)
	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return errors.New(fmt.Sprintf("Component '%s' does not exixt in %s", component, c.Config.TerraformDir))
	}

	// Print command info
	color.Cyan("Command info:")
	color.Green("Terraform command: " + subCommand)
	color.Green("Component: " + componentFromArg)
	if len(baseComponent) > 0 {
		color.Green("Base component: " + baseComponent)
	}
	color.Green("Stack: " + stack)
	color.Green("Additional arguments: %v\n", additionalArgsAndFlags)
	fmt.Println()

	// Execute command
	command := "terraform"
	color.Cyan(fmt.Sprintf("\nExecuting command: %s %s %s\n\n", command,
		subCommand, u.SliceOfStringsToSpaceSeparatedString(additionalArgsAndFlags)))

	err = execCommand(command, allArgsAndFlags)
	if err != nil {
		return err
	}

	return nil
}
