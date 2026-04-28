package terraform

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
)

// destroyParser handles flag parsing for the destroy command.
var destroyParser *flags.StandardParser

// destroyCmd represents the terraform destroy command.
var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy previously-created infrastructure",
	Long: `Destroy all the infrastructure managed by Terraform, removing resources as defined in the state file.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/destroy
  https://opentofu.org/docs/cli/commands/destroy`,
	RunE: func(cmd *cobra.Command, args []string) error {
		v := viper.GetViper()

		if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		if err := destroyParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := ParseTerraformRunOptions(v)
		return terraformRunWithOptions(terraformCmd, cmd, args, opts)
	},
}

func init() {
	destroyParser = flags.NewStandardParser(
		WithBackendExecutionFlags(),
		flags.WithBoolFlag("affected", "", false, "Destroy the affected components in reverse dependency order"),
		flags.WithBoolFlag("all", "", false, "Destroy all components in all stacks"),
	)

	destroyParser.RegisterFlags(destroyCmd)

	if err := destroyParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register completions for destroy command.
	RegisterTerraformCompletions(destroyCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "destroy", DestroyCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(destroyCmd)
}
