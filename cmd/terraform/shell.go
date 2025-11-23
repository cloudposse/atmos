package terraform

import (
	"github.com/spf13/cobra"
)

// shellCmd represents the terraform shell command (custom Atmos command).
var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Configure an environment for an Atmos component and start a new shell",
	Long: `Configure an environment for a specific Atmos component in a stack and then start a new shell.

In this shell, you can execute all native Terraform commands directly without the need
to use Atmos-specific arguments and flags. This allows you to interact with Terraform
as you would in a typical setup, but within the configured Atmos environment.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Register completions for shellCmd.
	RegisterTerraformCompletions(shellCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(shellCmd)
}
