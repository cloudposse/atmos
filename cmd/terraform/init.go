package terraform

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
)

// initParser handles flag parsing for init command.
var initParser *flags.StandardParser

// initCmd represents the terraform init command.
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Prepare your working directory for other commands",
	Long: `Initialize the working directory containing Terraform configuration files.

It will download necessary provider plugins and set up the backend.
Note: Atmos will automatically call init for you when running plan and apply commands.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/init
  https://opentofu.org/docs/cli/commands/init`,
	RunE: func(cmd *cobra.Command, args []string) error {
		v := viper.GetViper()

		// Bind both parent and subcommand parsers.
		if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		if err := initParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Parse base terraform options.
		opts := ParseTerraformRunOptions(v)

		return terraformRunWithOptions(terraformCmd, cmd, args, opts)
	},
}

func init() {
	// Create parser with init-specific flags.
	initParser = flags.NewStandardParser(
		WithBackendExecutionFlags(),
	)

	// Register init-specific flags with Cobra.
	initParser.RegisterFlags(initCmd)

	// Bind flags to Viper for environment variable support.
	if err := initParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register completions for initCmd.
	RegisterTerraformCompletions(initCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "init", InitCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(initCmd)
}
