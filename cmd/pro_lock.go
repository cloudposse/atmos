package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// proLockCmd executes 'pro lock' CLI command
var proLockCmd = &cobra.Command{
	Use:               "lock",
	Short:             "Lock a stack",
	Long:              `This command calls the atmos pro API and locks a stack`,
	Args:              cobra.NoArgs,
	ValidArgsFunction: ComponentsArgCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteProLockCommand(cmd, args)
		return err
	},
}

func init() {
	proLockCmd.PersistentFlags().StringP("component", "c", "", "Specify the Atmos component to lock")
	AddStackCompletion(proLockCmd)
	proLockCmd.PersistentFlags().StringP("message", "m", "", "The lock message to display if someone else tries to lock the stack. Defaults to `Locked by Atmos`")
	proLockCmd.PersistentFlags().Int32P("ttl", "t", 0, "The amount of time in seconds to lock the stack for. Defaults to 30")

	proCmd.AddCommand(proLockCmd)
}
