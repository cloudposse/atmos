package terraform

import (
	"github.com/spf13/cobra"

	h "github.com/cloudposse/atmos/pkg/hooks"
)

// deployCmd represents the terraform deploy command (custom Atmos command).
var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy the specified infrastructure using Terraform",
	Long: `Deploys infrastructure by running the Terraform apply command with automatic approval.

This ensures that the changes defined in your Terraform configuration are applied without requiring manual confirmation, streamlining the deployment process.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
	PostRunE: func(cmd *cobra.Command, args []string) error {
		return runHooks(h.AfterTerraformApply, cmd, args)
	},
}

func init() {
	// Command-specific flags for deploy
	deployCmd.PersistentFlags().Bool("deploy-run-init", false, "If set atmos will run `terraform init` before executing the command")
	deployCmd.PersistentFlags().Bool("from-plan", false, "If set atmos will use the previously generated plan file")
	deployCmd.PersistentFlags().String("planfile", "", "Set the plan file to use")
	deployCmd.PersistentFlags().Bool("affected", false, "Deploy the affected components in dependency order")
	deployCmd.PersistentFlags().Bool("all", false, "Deploy all components in all stacks")

	// Disable flag parsing to allow pass-through of Terraform native flags

	// Attach to parent terraform command
	// Register completions for deployCmd.
	RegisterTerraformCompletions(deployCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(deployCmd)
}
