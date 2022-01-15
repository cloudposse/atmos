package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"os"
)

// terraformShellCmd configures an environment for a component in a stack and starts a new shell allowing executing plain terraform commands
var terraformShellCmd = &cobra.Command{
	Use:                "shell",
	Short:              "Execute 'terraform shell' commands",
	Long:               "This command configures an environment for a component in a stack and starts a new shell allowing executing plain terraform commands",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteTerraformShell(cmd, args)
		if err != nil {
			color.Red("%s\n\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	terraformShellCmd.DisableFlagParsing = false
	terraformShellCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform shell <component> -s <stack>")

	err := terraformShellCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		color.Red("%s\n\n", err)
		os.Exit(1)
	}

	terraformCmd.AddCommand(terraformShellCmd)
}
