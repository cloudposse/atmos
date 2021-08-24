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
	"strings"
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
	var selectedStackConfigFile string
	var componentVarsSection map[interface{}]interface{}
	var baseComponent string
	var command string

	if c.Config.StackType == "Directory" {
		selectedStackConfigFile = c.Config.StackConfigFiles[0]

		componentVarsSection, baseComponent, command, err = checkStackConfig(stack, stacksMap, selectedStackConfigFile, componentFromArg)
		if err != nil {
			return err
		}

		if len(command) > 0 {
			color.Cyan("Found 'command=%s' for component '%s' in stack '%s'\n\n", command, componentFromArg, stack)
		} else {
			command = "terraform"
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

	color.Cyan("Variables for component '%s' in stack '%s':", componentFromArg, stack)
	err = u.PrintAsYAML(componentVarsSection)
	if err != nil {
		return err
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

	// Write variables to a file
	stackNameFormatted := strings.Replace(stack, "/", "-", -1)
	varFileName := fmt.Sprintf("%s/%s/%s-%s.terraform.tfvars.json", c.Config.TerraformDir, component, stackNameFormatted, componentFromArg)
	color.Cyan("Writing variables to file %s", varFileName)
	err = u.WriteToFileAsJSON(varFileName, componentVarsSection, 0644)
	if err != nil {
		return err
	}

	// Print command info
	color.Cyan("\nCommand info:")
	color.Green("Terraform binary: " + command)
	color.Green("Terraform command: " + subCommand)
	color.Green("Additional arguments: %v", additionalArgsAndFlags)
	color.Green("Component: " + componentFromArg)
	if len(baseComponent) > 0 {
		color.Green("Base component: " + baseComponent)
	}
	color.Green("Stack: " + stack)
	color.Green("Stack config file: " + u.TrimBasePathFromPath(c.Config.StacksBaseAbsolutePath+"/", selectedStackConfigFile))
	fmt.Println()

	// Execute command
	emoji, err := u.UnquoteCodePoint("\\U+1F680")
	if err != nil {
		return err
	}

	color.Cyan(fmt.Sprintf("\nExecuting command  %v", emoji))
	color.Green(fmt.Sprintf("Command: %s %s %s",
		command,
		subCommand,
		u.SliceOfStringsToSpaceSeparatedString(additionalArgsAndFlags)),
	)
	color.Green(fmt.Sprintf("Working dir: %s", componentPath))
	fmt.Println(strings.Repeat("\n", 2))

	err = execCommand(command, allArgsAndFlags, componentPath)
	if err != nil {
		return err
	}

	return nil
}
