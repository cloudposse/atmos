package exec

import (
	"github.com/spf13/cobra"

	tui "github.com/cloudposse/atmos/internal/tui/stack_component_select"
)

func ExecuteExecCmd(cmd *cobra.Command, args []string) error {
	if err := tui.Execute(); err != nil {
		return err
	}

	if _, err := ExecuteDescribeComponent("vpc", "plat-ue2-dev"); err != nil {
		return err
	}

	return nil
}
