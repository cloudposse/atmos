package terraform

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
)

// refreshParser handles flag parsing for refresh command.
var refreshParser *flags.StandardParser

// refreshCmd represents the terraform refresh command.
var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Update the state to match remote systems",
	Long: `Refresh the Terraform state, reconciling the local state with the actual infrastructure state.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/refresh
  https://opentofu.org/docs/cli/commands/refresh`,
	RunE: func(cmd *cobra.Command, args []string) error {
		v := viper.GetViper()

		// Bind both parent and subcommand parsers.
		if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		if err := refreshParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Parse base terraform options with command context for UI flag detection.
		opts := ParseTerraformRunOptions(v, cmd)

		return terraformRunWithOptions(terraformCmd, cmd, args, opts)
	},
}

func init() {
	// Create parser with refresh-specific flags (backend execution for init).
	refreshParser = flags.NewStandardParser(
		WithBackendExecutionFlags(),
	)

	// Register refresh-specific flags with Cobra.
	refreshParser.RegisterFlags(refreshCmd)

	// Bind flags to Viper for environment variable support.
	if err := refreshParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register completions for refreshCmd.
	RegisterTerraformCompletions(refreshCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "refresh", RefreshCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(refreshCmd)
}
