package terraform

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
)

// planParser handles flag parsing for plan command.
var planParser *flags.StandardParser

// planCmd represents the terraform plan command.
var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Show changes required by the current configuration",
	Long: `Generate an execution plan, which shows what actions Terraform will take to reach the desired state of the configuration.

This command shows what Terraform will do when you run 'apply'. It helps you verify changes before making them to your infrastructure.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/plan
  https://opentofu.org/docs/cli/commands/plan`,
	RunE: func(cmd *cobra.Command, args []string) error {
		v := viper.GetViper()

		// Bind both parent and subcommand parsers.
		if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		if err := planParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Parse base terraform options.
		opts := ParseTerraformRunOptions(v)

		// Plan-specific flags (upload-status, skip-planfile) flow through the
		// legacy ProcessCommandLineArgs which sets info.PlanSkipPlanfile.
		// The Viper binding above ensures flag > env > config precedence works.

		return terraformRunWithOptions(terraformCmd, cmd, args, opts)
	},
}

func init() {
	// Create parser with plan-specific flags using functional options.
	planParser = flags.NewStandardParser(
		flags.WithBoolFlag("upload-status", "", false, "If set atmos will upload the plan result to the pro API"),
		flags.WithBoolFlag("affected", "", false, "Plan the affected components in dependency order"),
		flags.WithBoolFlag("all", "", false, "Plan all components in all stacks"),
		flags.WithBoolFlag("skip-planfile", "", false, "Skip writing the plan to a file by not passing the `-out` flag to Terraform"),
		flags.WithEnvVars("upload-status", "ATMOS_TERRAFORM_PLAN_UPLOAD_STATUS"),
		flags.WithEnvVars("skip-planfile", "ATMOS_TERRAFORM_PLAN_SKIP_PLANFILE"),
	)

	// Register plan-specific flags with Cobra.
	planParser.RegisterFlags(planCmd)

	// Bind flags to Viper for environment variable support.
	if err := planParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register completions for plan command.
	RegisterTerraformCompletions(planCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "plan", PlanCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(planCmd)
}
