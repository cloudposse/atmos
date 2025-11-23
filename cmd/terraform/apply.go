package terraform

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	h "github.com/cloudposse/atmos/pkg/hooks"
)

// applyRegistry holds apply-specific flags.
var applyRegistry *flags.FlagRegistry

// applyCmd represents the terraform apply command.
var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply changes to infrastructure",
	Long: `Apply the changes required to reach the desired state of the configuration.

This will prompt for confirmation before making changes unless -auto-approve is used.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/apply
  https://opentofu.org/docs/cli/commands/apply`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
	PostRunE: func(cmd *cobra.Command, args []string) error {
		return runHooks(h.AfterTerraformApply, cmd, args)
	},
}

func init() {
	// Create flag registry for apply-specific flags.
	applyRegistry = flags.NewFlagRegistry()
	applyRegistry.Register(&flags.BoolFlag{
		Name:        "from-plan",
		Description: "If set atmos will use the previously generated plan file",
	})
	applyRegistry.Register(&flags.StringFlag{
		Name:        "planfile",
		Description: "Set the plan file to use",
	})
	applyRegistry.Register(&flags.BoolFlag{
		Name:        "affected",
		Description: "Apply the affected components in dependency order",
	})
	applyRegistry.Register(&flags.BoolFlag{
		Name:        "all",
		Description: "Apply all components in all stacks",
	})

	// Register apply-specific flags with Cobra.
	applyRegistry.RegisterFlags(applyCmd)

	// Bind flags to Viper.
	if err := applyRegistry.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Set custom help to show terraform native flags.
	setCustomHelp(applyCmd, ApplyCompatFlagDescriptions())

	// Register completions for apply command.
	RegisterTerraformCompletions(applyCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(applyCmd)
}
