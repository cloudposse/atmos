package terraform

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
)

// workspaceCmd represents the terraform workspace command.
var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Manage Terraform workspaces",
	Long: `Manage Terraform workspaces for organizing multiple states within a single configuration.

The 'atmos terraform workspace' command initializes Terraform for the current configuration,
selects the specified workspace, and creates it if it does not already exist.

It runs the following sequence of Terraform commands:
1. 'terraform init -reconfigure' to initialize the working directory.
2. 'terraform workspace select' to switch to the specified workspace.
3. If the workspace does not exist, it runs 'terraform workspace new' to create and select it.

This ensures that the workspace is properly set up for Terraform operations.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/workspace
  https://opentofu.org/docs/cli/commands/workspace`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Register completions for workspaceCmd.
	RegisterTerraformCompletions(workspaceCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "workspace", WorkspaceCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(workspaceCmd)
}
