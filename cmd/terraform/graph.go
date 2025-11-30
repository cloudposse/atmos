package terraform

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
)

// graphCmd represents the terraform graph command.
var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Generate a Graphviz graph of the steps in an operation",
	Long: `Outputs the visual execution graph of Terraform resources.

The output is in the DOT format, which can be used by GraphViz to generate charts.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/graph
  https://opentofu.org/docs/cli/commands/graph`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Register completions for graphCmd.
	RegisterTerraformCompletions(graphCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "graph", GraphCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(graphCmd)
}
