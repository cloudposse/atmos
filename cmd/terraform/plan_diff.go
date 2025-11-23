package terraform

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
)

// planDiffCmd represents the terraform plan-diff command (custom Atmos command).
var planDiffCmd = &cobra.Command{
	Use:   "plan-diff",
	Short: "Compare two Terraform plans and show the differences",
	Long: `Compare two Terraform plans and show the differences between them.

It takes an original plan file (--orig) and optionally a new plan file (--new).
If the new plan file is not provided, it will generate one by running 'terraform plan'
with the current configuration.

The command shows differences in variables, resources, and outputs between the two plans.

Example usage:
  atmos terraform plan-diff myapp -s dev --orig=orig.plan
  atmos terraform plan-diff myapp -s dev --orig=orig.plan --new=new.plan`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Command-specific flags for plan-diff
	planDiffCmd.PersistentFlags().String("orig", "", "Path to the original Terraform plan file (required)")
	planDiffCmd.PersistentFlags().String("new", "", "Path to the new Terraform plan file (optional)")
	err := planDiffCmd.MarkPersistentFlagRequired("orig")
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "Error marking 'orig' flag as required", "")
	}

	// Register completions for planDiffCmd.
	RegisterTerraformCompletions(planDiffCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(planDiffCmd)
}
