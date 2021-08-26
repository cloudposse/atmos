// https://github.com/roboll/helmfile#cli-reference

package exec

import (
	c "atmos/internal/config"
	u "atmos/internal/utils"
	"fmt"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"path"
	"strings"
)

// ExecuteHelmfile executes helmfile commands
func ExecuteHelmfile(cmd *cobra.Command, args []string) error {
	stack, componentFromArg, component, baseComponent, command, subCommand, componentVarsSection, additionalArgsAndFlags,
		err := processConfigAndStacks("helmfile", cmd, args)

	componentPath := path.Join(c.ProcessedConfig.HelmfileDirAbsolutePath, component)
	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return errors.New(fmt.Sprintf("Component '%s' does not exixt in %s", component, c.ProcessedConfig.HelmfileDirAbsolutePath))
	}

	// Write variables to a file
	stackNameFormatted := strings.Replace(stack, "/", "-", -1)
	varFileName := fmt.Sprintf("%s/%s/%s-%s.helmfile.vars.yaml", c.Config.Components.Helmfile.BasePath, component, stackNameFormatted, componentFromArg)
	color.Cyan("Writing variables to file:")
	fmt.Println(varFileName)
	err = u.WriteToFileAsYAML(varFileName, componentVarsSection, 0644)
	if err != nil {
		return err
	}

	// Handle `helmfile deploy` custom command
	if subCommand == "deploy" {
		subCommand = "sync"
	}

	// Print command info
	color.Cyan("\nCommand info:")
	color.Green("Helmfile binary: " + command)
	color.Green("Helmfile command: " + subCommand)
	color.Green("Arguments and flags: %v", additionalArgsAndFlags)
	color.Green("Component: " + componentFromArg)
	if len(baseComponent) > 0 {
		color.Green("Base component: " + baseComponent)
	}
	color.Green("Stack: " + stack)
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
		u.SliceOfStringsToSpaceSeparatedString(additionalArgsAndFlags),
	))

	workingDir := fmt.Sprintf("%s/%s", c.Config.Components.Helmfile.BasePath, component)
	color.Green(fmt.Sprintf("Working dir: %s", workingDir))
	fmt.Println(strings.Repeat("\n", 2))

	varFile := fmt.Sprintf("%s-%s.helmfile.vars.yaml", stackNameFormatted, componentFromArg)

	allArgsAndFlags := []string{"--state-values-file", varFile}
	allArgsAndFlags = append(allArgsAndFlags, subCommand)
	allArgsAndFlags = append(allArgsAndFlags, additionalArgsAndFlags...)

	// Execute the command
	err = execCommand(command, allArgsAndFlags, componentPath, nil)
	if err != nil {
		return err
	}

	// Cleanup
	varFilePath := fmt.Sprintf("%s/%s/%s", c.ProcessedConfig.HelmfileDirAbsolutePath, component, varFile)
	err = os.Remove(varFilePath)
	if err != nil {
		color.Red("Error deleting helmfile var file: %s\n", err)
	}

	return nil
}
