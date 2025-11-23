package terraform

import (
	"github.com/spf13/cobra"
)

// cleanCmd represents the terraform clean command (custom Atmos command).
var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean up Terraform state and artifacts",
	Long: `Remove temporary files, state locks, and other artifacts created during Terraform operations.

This helps reset the environment and ensures no leftover data interferes with subsequent runs.

Common use cases:
- Releasing locks on Terraform state files.
- Cleaning up temporary workspaces or plans.
- Preparing the environment for a fresh deployment.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Command-specific flags for clean
	cleanCmd.PersistentFlags().Bool("everything", false, "If set atmos will also delete the Terraform state files and directories for the component")
	cleanCmd.PersistentFlags().Bool("force", false, "Forcefully delete Terraform state files and directories without interaction")
	cleanCmd.PersistentFlags().Bool("skip-lock-file", false, "Skip deleting the `.terraform.lock.hcl` file")

	// Register completions for cleanCmd.
	RegisterTerraformCompletions(cleanCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(cleanCmd)
}
