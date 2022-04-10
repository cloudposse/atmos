package exec

import (
	"errors"
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// ExecuteHelmfileGenerateVarfile executes `helmfile generate varfile` command
func ExecuteHelmfileGenerateVarfile(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument `component`")
	}

	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	component := args[0]

	var info c.ConfigAndStacksInfo
	info.ComponentFromArg = component
	info.Stack = stack
	info.ComponentType = "helmfile"

	info, err = ProcessStacks(info)
	if err != nil {
		return err
	}

	var varFileNameFromArg string
	var varFilePath string

	varFileNameFromArg, err = flags.GetString("file")
	if err != nil {
		varFileNameFromArg = ""
	}

	if len(varFileNameFromArg) > 0 {
		varFilePath = varFileNameFromArg
	} else {
		varFilePath = constructHelmfileComponentVarfilePath(info)
	}

	// Print the component variables
	color.Cyan("\nVariables for the component '%s' in the stack '%s':\n\n", info.ComponentFromArg, info.Stack)
	err = utils.PrintAsYAML(info.ComponentVarsSection)
	if err != nil {
		return err
	}

	// Write the variables to file
	color.Cyan("Writing the variables to file:")
	fmt.Println(varFilePath)

	if !info.DryRun {
		err = utils.WriteToFileAsYAML(varFilePath, info.ComponentVarsSection, 0644)
		if err != nil {
			return err
		}
	}

	fmt.Println()
	return nil
}
