package terraform

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
)

// planRegistry holds plan-specific flags.
var planRegistry *flags.FlagRegistry

// planCmd represents the terraform plan command.
var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Show changes required by the current configuration",
	Long: `Generate an execution plan, which shows what actions Terraform will take to reach the desired state of the configuration.

This command shows what Terraform will do when you run 'apply'. It helps you verify changes before making them to your infrastructure.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/plan
  https://opentofu.org/docs/cli/commands/plan`,
	// FParseErrWhitelist allows unknown flags to pass through to Terraform/OpenTofu.
	// The AtmosFlagParser handles separation of Atmos flags from terraform native flags.
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Create flag registry for plan-specific flags.
	planRegistry = flags.NewFlagRegistry()
	planRegistry.Register(&flags.BoolFlag{
		Name:        "upload-status",
		Description: "If set atmos will upload the plan result to the pro API",
	})
	planRegistry.Register(&flags.BoolFlag{
		Name:        "affected",
		Description: "Plan the affected components in dependency order",
	})
	planRegistry.Register(&flags.BoolFlag{
		Name:        "all",
		Description: "Plan all components in all stacks",
	})
	planRegistry.Register(&flags.BoolFlag{
		Name:        "skip-planfile",
		Description: "Skip writing the plan to a file by not passing the `-out` flag to Terraform",
	})

	// Register plan-specific flags with Cobra.
	planRegistry.RegisterFlags(planCmd)

	// Bind flags to Viper.
	if err := planRegistry.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Set custom help to show terraform native flags.
	setCustomHelp(planCmd, PlanCompatFlagDescriptions())

	// Register completions for plan command.
	RegisterTerraformCompletions(planCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(planCmd)
}
