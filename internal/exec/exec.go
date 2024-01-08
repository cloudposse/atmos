package exec

import (
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"

	tui "github.com/cloudposse/atmos/internal/tui/stack_component_select"
)

func ExecuteExecCmd(cmd *cobra.Command, args []string) error {
	component, stack, err := tui.Execute()
	if err != nil {
		return err
	}

	c, err := ExecuteDescribeComponent(component, stack)
	if err != nil {
		return err
	}

	err = u.PrintAsYAML(c)
	if err != nil {
		return err
	}

	return nil
}
