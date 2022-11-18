package cmd

import (
	"github.com/spf13/cobra"
	"os"

	u "github.com/cloudposse/atmos/pkg/utils"
)

var completionCmd = &cobra.Command{
	Use:                   "completion [bash|zsh|fish|powershell]",
	Short:                 "Generate completion script for Bash, Zsh, fish or PowerShell",
	Long:                  "This command generates completion scripts for Bash, Zsh, fish and PowerShell",
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			err := cmd.Root().GenBashCompletion(os.Stdout)
			if err != nil {
				u.PrintErrorToStdErrorAndExit(err)
			}
		case "zsh":
			err := cmd.Root().GenZshCompletion(os.Stdout)
			if err != nil {
				u.PrintErrorToStdErrorAndExit(err)
			}
		case "fish":
			err := cmd.Root().GenFishCompletion(os.Stdout, true)
			if err != nil {
				u.PrintErrorToStdErrorAndExit(err)
			}
		case "powershell":
			err := cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			if err != nil {
				u.PrintErrorToStdErrorAndExit(err)
			}
		}
	},
}

func init() {
	RootCmd.AddCommand(completionCmd)
}
