package terraform

import (
	"github.com/spf13/cobra"
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
	// Set custom help to show terraform native flags
	setCustomHelp(graphCmd, GraphCompatFlagDescriptions())

	// Register completions for graphCmd.
	RegisterTerraformCompletions(graphCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(graphCmd)
}
