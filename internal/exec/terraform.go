package exec

import (
	c "atmos/internal/config"
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
	//_, stacksMap, err := s.ProcessYAMLConfigFiles(
	//	c.Config.StacksBaseAbsolutePath,
	//	c.Config.StackConfigFiles,
	//	false,
	//	true)

	if err != nil {
		return err
	}

	// Find and check component
	component := args[1]
	if len(component) < 1 {
		return errors.New("'component' is required")
	}
	componentPath := path.Join(c.Config.TerraformDirAbsolutePath, component)
	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return errors.New(fmt.Sprintf("Component '%s' does not exixt at '%s'", component, componentPath))
	}

	color.Green(strings.Repeat("-", 120))
	color.Green("Terraform command: " + subCommand)
	color.Green("Component: " + component)
	color.Green("Stack: " + stack)
	color.Green("Additional arguments: %v\n", additionalArgsAndFlags)
	color.Green(strings.Repeat("-", 120))

	//yamlConfig, err := yaml.Marshal(stacksMap)
	//if err != nil {
	//	return err
	//}
	//fmt.Printf(string(yamlConfig))

	// Execute command
	command := "terraform"
	color.Blue(fmt.Sprintf("\nExecuting command: %s %s %s\n", command,
		subCommand, u.SliceOfStringsToSpaceSeparatedString(additionalArgsAndFlags)))

	err = execCommand(command, allArgsAndFlags)
	if err != nil {
		return err
	}

	return nil
}
