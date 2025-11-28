package terraform

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

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
		return terraformRun(terraformCmd, cmd, args)
	},
	PostRunE: func(cmd *cobra.Command, args []string) error {
		return runHooks(h.AfterTerraformApply, cmd, args)
	},
}

func init() {
	// Create parser with apply-specific flags using functional options.
	applyParser = flags.NewStandardParser(
		flags.WithBoolFlag("from-plan", "", false, "If set atmos will use the previously generated plan file"),
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

	// Set custom help to show terraform native flags.
	setCustomHelp(applyCmd, ApplyCompatFlagDescriptions())

	// Register completions for apply command.
	RegisterTerraformCompletions(applyCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(applyCmd)
}
