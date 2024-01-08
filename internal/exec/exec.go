package exec

import (
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"

	tui "github.com/cloudposse/atmos/internal/tui/stack_component_select"
)

func ExecuteExecCmd(cmd *cobra.Command, args []string) error {
	if err := tui.Execute(); err != nil {
		return err
	}

	c, err := ExecuteDescribeComponent("vpc", "plat-ue2-dev")
	if err != nil {
		return err
	}

	err = u.PrintAsYAML(c)
	if err != nil {
		return err
	}

	return nil
}
