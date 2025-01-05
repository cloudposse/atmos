package cmd

import (
	"os"

	"github.com/spf13/cobra"

	u "github.com/cloudposse/atmos/pkg/utils"
)

var completionCmd = &cobra.Command{
	Use:                   "completion [bash|zsh|fish|powershell]",
	Short:                 "Generate completion script for Bash, Zsh, Fish and PowerShell",
	Long:                  "This command generates completion scripts for Bash, Zsh, Fish and PowerShell",
	DisableFlagsInUseLine: true,
	// Why I am not using cobra inbuilt validation for Args:
	// Because we have our own custom validation for Args
	// Why we have our own custom validation for Args:
	// Because we want to show custom error message when user provides invalid shell name
	// ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	// Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
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
		u.LogErrorAndExit(atmosConfig, err)
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
		})
	}
	addUsageCommand(completionCmd, false)
	RootCmd.AddCommand(completionCmd)
}
