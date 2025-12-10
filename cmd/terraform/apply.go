package terraform

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	h "github.com/cloudposse/atmos/pkg/hooks"
)

// applyParser handles flag parsing for apply command.
var applyParser *flags.StandardParser

// applyCmd represents the terraform apply command.
var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply changes to infrastructure",
	Long: `Apply the changes required to reach the desired state of the configuration.

This will prompt for confirmation before making changes unless -auto-approve is used.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/apply
  https://opentofu.org/docs/cli/commands/apply`,
	RunE: func(cmd *cobra.Command, args []string) error {
		v := viper.GetViper()

		// Bind both parent and subcommand parsers.
		if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		if err := applyParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Parse base terraform options.
		opts := ParseTerraformRunOptions(v)

		// Apply-specific flags (from-plan, planfile) flow through the
		// legacy ProcessCommandLineArgs which sets info.UseTerraformPlan, info.PlanFile.
		// The Viper binding above ensures flag > env > config precedence works.

		return terraformRunWithOptions(terraformCmd, cmd, args, opts)
	},
	PostRunE: func(cmd *cobra.Command, args []string) error {
		return runHooks(h.AfterTerraformApply, cmd, args)
	},
}

func init() {
	// Create parser with apply-specific flags using functional options.
	applyParser = flags.NewStandardParser(
		flags.WithStringFlag("from-plan", "", "", "Apply from plan file (e.g., atmos terraform apply <component> -s <stack> --from-plan)"),
		flags.WithNoOptDefVal("from-plan", "true"),
		flags.WithStringFlag("planfile", "", "", "Set the plan file to use"),
		flags.WithBoolFlag("affected", "", false, "Apply the affected components in dependency order"),
		flags.WithBoolFlag("all", "", false, "Apply all components in all stacks"),
		flags.WithEnvVars("from-plan", "ATMOS_TERRAFORM_APPLY_FROM_PLAN"),
		flags.WithEnvVars("planfile", "ATMOS_TERRAFORM_APPLY_PLANFILE"),
	)

	// Register apply-specific flags with Cobra.
	applyParser.RegisterFlags(applyCmd)

	// Bind flags to Viper for environment variable support.
	if err := applyParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register completions for apply command.
	RegisterTerraformCompletions(applyCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "apply", ApplyCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(applyCmd)
}
