package cmd

import (
	"os"

	"github.com/spf13/cobra"

	u "github.com/cloudposse/atmos/pkg/utils"
)

var completionCmd = &cobra.Command{
	Use:                   "completion [bash|zsh|fish|powershell]",
	Short:                 "Generate autocompletion scripts for Bash, Zsh, Fish, and PowerShell",
	Long:                  "This command generates completion scripts for Bash, Zsh, Fish and PowerShell",
	DisableFlagsInUseLine: true,
	Args:                  cobra.NoArgs,
}

func runCompletion(cmd *cobra.Command, args []string) {
	var err error

	switch cmd.Use {
	case "bash":
		err = cmd.Root().GenBashCompletion(os.Stdout)
	case "zsh":
		err = cmd.Root().GenZshCompletion(os.Stdout)
	case "fish":
		err = cmd.Root().GenFishCompletion(os.Stdout, true)
	case "powershell":
		err = cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
	}

	if err != nil {
		u.LogErrorAndExit(err)
	}
}

func init() {
	shellNames := []string{"bash", "zsh", "fish", "powershell"}
	for _, shellName := range shellNames {
		completionCmd.AddCommand(&cobra.Command{
			Use:   shellName,
			Short: "Generate completion script for " + shellName,
			Long:  "This command generates completion scripts for " + shellName,
			Run:   runCompletion,
			Args:  cobra.NoArgs,
		})
	}

	RootCmd.AddCommand(completionCmd)
}
