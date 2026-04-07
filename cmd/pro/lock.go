package pro

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// lockCmd executes 'pro lock' CLI command.
var lockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Lock a stack",
	Long:  `This command calls the atmos pro API and locks a stack`,
	Args:  cobra.NoArgs,
	RunE:  e.ExecuteProLockCommand,
}

func init() {
	lockCmd.PersistentFlags().StringP("component", "c", "", "Specify the Atmos component to lock")
	lockCmd.PersistentFlags().StringP("stack", "s", "", "Specify the Atmos stack to lock")
	lockCmd.PersistentFlags().StringP("message", "m", "", `Lock message displayed when someone else tries to lock the stack (default "Locked by Atmos")`)
	lockCmd.PersistentFlags().Int32P("ttl", "t", 0, "Time in seconds to lock the stack for (default 30)")
}
