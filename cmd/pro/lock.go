package pro

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

const (
	// DefaultLockMessage is the default message shown when a stack is locked.
	defaultLockMessage = "Locked by Atmos"
	// DefaultLockTTL is the default lock duration in seconds.
	defaultLockTTL = 30
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
	lockCmd.PersistentFlags().StringP("message", "m", defaultLockMessage, "Lock message displayed when someone else tries to lock the stack")
	lockCmd.PersistentFlags().Int32P("ttl", "t", defaultLockTTL, "Time in seconds to lock the stack for")
}
