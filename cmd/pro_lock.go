package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// proLockCmd executes 'pro lock' CLI command
var proLockCmd = &cobra.Command{
	Use:                "lock",
	Short:              "Lock a stack",
	Long:               `This command calls the atmos pro API and locks a stack`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteProLockCommand(cmd, args)
		if err != nil {
			u.LogErrorAndExit(err)
		}
	},
}

func init() {
	proLockCmd.PersistentFlags().StringP("component", "c", "", "Only vendor the specified component: atmos vendor pull --component <component>")
	proLockCmd.PersistentFlags().StringP("stack", "s", "", "Only vendor the specified stack: atmos vendor pull --stack <stack>")
	proLockCmd.PersistentFlags().StringP("message", "m", "", "The lock message to display i someone else tries to lock the stack. Defaults to 'Locked by Atmos'")
	proLockCmd.PersistentFlags().Int32P("ttl", "t", 0, "The amount of time in seconds to lock the stack for. Defaults to 30")

	proCmd.AddCommand(proLockCmd)
}
