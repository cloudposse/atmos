package terraform

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
)

// forceUnlockCmd represents the terraform force-unlock command.
var forceUnlockCmd = &cobra.Command{
	Use:   "force-unlock",
	Short: "Release a stuck lock on the current workspace",
	Long: `Manually unlock the state for the defined configuration.

This will not modify your infrastructure. This command removes the lock on the state for the current configuration.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/force-unlock
  https://opentofu.org/docs/cli/commands/force-unlock`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Register completions for forceUnlockCmd.
	RegisterTerraformCompletions(forceUnlockCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "force-unlock", ForceUnlockCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(forceUnlockCmd)
}
