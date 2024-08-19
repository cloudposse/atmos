package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// proUnlockCmd executes 'pro unlock' CLI command
var proUnlockCmd = &cobra.Command{
	Use:                "unlock",
	Short:              "Unlock a stack",
	Long:               `This command calls the atmos pro API and unlocks a stack`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteProUnlockCommand(cmd, args)
		if err != nil {
			u.LogErrorAndExit(err)
		}
	},
}

func init() {
	proUnlockCmd.PersistentFlags().StringP("component", "c", "", "Only vendor the specified component: atmos vendor pull --component <component>")
	proUnlockCmd.PersistentFlags().StringP("stack", "s", "", "Only vendor the specified stack: atmos vendor pull --stack <stack>")

	proCmd.AddCommand(proUnlockCmd)
}
