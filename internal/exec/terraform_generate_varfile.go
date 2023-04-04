package exec

import (
	"errors"
	"fmt"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteTerraformGenerateVarfileCmd executes `terraform generate varfile` command
func ExecuteTerraformGenerateVarfileCmd(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument `component`")
	}

	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	component := args[0]

	info, err := processCommandLineArgs("terraform", cmd, args)
	if err != nil {
		return err
	}

	info.ComponentFromArg = component
	info.Stack = stack
	info.ComponentType = "terraform"

	cliConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	info, err = ProcessStacks(cliConfig, info, true)
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
		varFilePath = constructTerraformComponentVarfilePath(cliConfig, info)
	}

	// Print the component variables
	u.LogInfo(cliConfig, fmt.Sprintf("\nVariables for the component '%s' in the stack '%s':\n", info.ComponentFromArg, info.Stack))
	err = u.PrintAsYAML(cliConfig, info.ComponentVarsSection)
	if err != nil {
		return err
	}

	// Write the variables to file
	u.LogInfo(cliConfig, "Writing the variables to file:")
	u.LogMessage(cliConfig, varFilePath)

	if !info.DryRun {
		err = u.WriteToFileAsJSON(varFilePath, info.ComponentVarsSection, 0644)
		if err != nil {
			return err
		}
	}

	return nil
}
