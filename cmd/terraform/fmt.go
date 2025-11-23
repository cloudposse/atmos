package terraform

import (
	"github.com/spf13/cobra"
)

// fmtCmd represents the terraform fmt command.
var fmtCmd = &cobra.Command{
	Use:   "fmt",
	Short: "Reformat your configuration in the standard style",
	Long: `Rewrite Terraform configuration files to a canonical format and style.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/fmt
  https://opentofu.org/docs/cli/commands/fmt`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Set custom help to show terraform native flags
	setCustomHelp(fmtCmd, FmtCompatFlagDescriptions())

	// Register completions for fmtCmd.
	RegisterTerraformCompletions(fmtCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(fmtCmd)
}
