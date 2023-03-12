package exec

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteDescribeDependantsCmd executes `describe dependants` command
func ExecuteDescribeDependantsCmd(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument `component`")
	}

	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	format, err := flags.GetString("format")
	if err != nil {
		return err
	}

	file, err := flags.GetString("file")
	if err != nil {
		return err
	}

	component := args[0]

	componentSection, err := ExecuteDescribeDependants(component, stack)
	if err != nil {
		return err
	}

	fmt.Println()
	err = printOrWriteToFile(format, file, componentSection)
	if err != nil {
		return err
	}

	return nil
}

// ExecuteDescribeDependants produces a list of Atmos components in Atmos stacks that depend on the provided Atmos component
func ExecuteDescribeDependants(component string, stack string) (map[string]any, error) {
	var configAndStacksInfo cfg.ConfigAndStacksInfo
	configAndStacksInfo.ComponentFromArg = component
	configAndStacksInfo.Stack = stack

	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	configAndStacksInfo.ComponentType = "terraform"
	configAndStacksInfo, err = ProcessStacks(cliConfig, configAndStacksInfo, true)
	if err != nil {
		u.PrintErrorVerbose(cliConfig.Logs.Verbose, err)
		configAndStacksInfo.ComponentType = "helmfile"
		configAndStacksInfo, err = ProcessStacks(cliConfig, configAndStacksInfo, true)
		if err != nil {
			return nil, err
		}
	}

	return configAndStacksInfo.ComponentSection, nil
}
