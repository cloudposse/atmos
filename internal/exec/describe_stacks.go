package exec

import (
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// ExecuteDescribeStacks executes `describe stacks` command
func ExecuteDescribeStacks(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	var configAndStacksInfo c.ConfigAndStacksInfo
	configAndStacksInfo.Stack = stack

	stacksMap, err := FindStacksMap(configAndStacksInfo)
	if err != nil {
		return err
	}

	err = u.PrintAsYAML(stacksMap)
	if err != nil {
		return err
	}

	return nil
}
