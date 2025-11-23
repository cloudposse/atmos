package terraform

import (
	"github.com/spf13/cobra"
)

// modulesCmd represents the terraform modules command.
var modulesCmd = &cobra.Command{
	Use:   "modules",
	Short: "Show all declared modules in a working directory",
	Long: `List all the modules declared in the current working directory.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/modules
  https://opentofu.org/docs/cli/commands/modules`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Register completions for modulesCmd.
	RegisterTerraformCompletions(modulesCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(modulesCmd)
}
