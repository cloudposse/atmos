package exec

import (
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// ExecuteDescribeStacks executes `describe stacks` command
func ExecuteDescribeStacks(cmd *cobra.Command, args []string) error {
	var configAndStacksInfo c.ConfigAndStacksInfo

	stacksMap, err := FindStacksMap(configAndStacksInfo, false)
	if err != nil {
		return err
	}

	err = u.PrintAsYAML(stacksMap)
	if err != nil {
		return err
	}

	return nil
}
