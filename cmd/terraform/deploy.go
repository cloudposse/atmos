package terraform

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	h "github.com/cloudposse/atmos/pkg/hooks"
)

// deployParser handles flag parsing for deploy command.
var deployParser *flags.StandardParser

// deployCmd represents the terraform deploy command (custom Atmos command).
var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy the specified infrastructure using Terraform",
	Long: `Deploys infrastructure by running the Terraform apply command with automatic approval.

This ensures that the changes defined in your Terraform configuration are applied without requiring manual confirmation, streamlining the deployment process.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		v := viper.GetViper()

		// Bind both parent and subcommand parsers.
		if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		if err := deployParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Parse base terraform options.
		opts := ParseTerraformRunOptions(v)

		// Deploy-specific flags (deploy-run-init, from-plan, planfile) flow through
		// the legacy ProcessCommandLineArgs which sets info.DeployRunInit, etc.
		// The Viper binding above ensures flag > env > config precedence works.

		return terraformRunWithOptions(terraformCmd, cmd, args, opts)
	},
	PostRunE: func(cmd *cobra.Command, args []string) error {
		return runHooks(h.AfterTerraformApply, cmd, args)
	},
}

func init() {
	// Create parser with deploy-specific flags using functional options.
	deployParser = flags.NewStandardParser(
		flags.WithBoolFlag("deploy-run-init", "", false, "If set atmos will run `terraform init` before executing the command"),
		flags.WithStringFlag("from-plan", "", "", "Apply from plan file (e.g., atmos terraform deploy <component> -s <stack> --from-plan)"),
		flags.WithNoOptDefVal("from-plan", "true"),
		flags.WithStringFlag("planfile", "", "", "Set the plan file to use"),
		flags.WithBoolFlag("affected", "", false, "Deploy the affected components in dependency order"),
		flags.WithBoolFlag("all", "", false, "Deploy all components in all stacks"),
		flags.WithEnvVars("deploy-run-init", "ATMOS_TERRAFORM_DEPLOY_RUN_INIT"),
		flags.WithEnvVars("from-plan", "ATMOS_TERRAFORM_DEPLOY_FROM_PLAN"),
		flags.WithEnvVars("planfile", "ATMOS_TERRAFORM_DEPLOY_PLANFILE"),
	)

	// Register deploy-specific flags with Cobra.
	deployParser.RegisterFlags(deployCmd)

	// Bind flags to Viper for environment variable support.
	if err := deployParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register completions for deployCmd.
	RegisterTerraformCompletions(deployCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(deployCmd)
}
