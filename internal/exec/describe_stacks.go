package exec

import (
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	g "github.com/cloudposse/atmos/pkg/globals"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// ExecuteDescribeStacks executes `describe stacks` command
func ExecuteDescribeStacks(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument `component`")
	}

	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	component := args[0]

	var configAndStacksInfo c.ConfigAndStacksInfo
	configAndStacksInfo.ComponentFromArg = component
	configAndStacksInfo.Stack = stack

	configAndStacksInfo.ComponentType = "terraform"
	configAndStacksInfo, err = ProcessStacks(configAndStacksInfo)
	if err != nil {
		configAndStacksInfo.ComponentType = "helmfile"
		configAndStacksInfo, err = ProcessStacks(configAndStacksInfo)
		if err != nil {
			return err
		}
	}

	if g.LogVerbose {
		fmt.Println()
		color.Cyan("Component config:\n\n")
	}

	err = u.PrintAsYAML(configAndStacksInfo.ComponentSection)
	if err != nil {
		return err
	}

	return nil
}
