package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeStacksCmd describes configuration for stacks and components in the stacks
var describeStacksCmd = &cobra.Command{
	Use:                "stacks",
	Short:              "Display configuration for Atmos stacks and their components",
	Long:               "This command shows the configuration details for Atmos stacks and the components within those stacks.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteDescribeStacksCmd(cmd, args)
		if err != nil {
			u.PrintErrorMarkdownAndExit("", err, "")
		}
	},
}

func init() {
	describeStacksCmd.DisableFlagParsing = false

	describeStacksCmd.PersistentFlags().String("file", "", "Write the result to file")

	describeStacksCmd.PersistentFlags().String("format", "yaml", "Specify the output format (`yaml` is default)")

	describeStacksCmd.PersistentFlags().StringP("stack", "s", "",
		"Filter by a specific stack\n"+
			"The filter supports names of the top-level stack manifests (including subfolder paths), and `atmos` stack names (derived from the context vars)",
	)
	AddStackCompletion(describeStacksCmd)
	describeStacksCmd.PersistentFlags().String("components", "", "Filter by specific `atmos` components")

	describeStacksCmd.PersistentFlags().String("component-types", "", "Filter by specific component types. Supported component types: terraform, helmfile")

	describeStacksCmd.PersistentFlags().String("sections", "", "Output only the specified component sections. Available component sections: `backend`, `backend_type`, `deps`, `env`, `inheritance`, `metadata`, `remote_state_backend`, `remote_state_backend_type`, `settings`, `vars`")

	describeStacksCmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")

	describeStacksCmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")

	describeStacksCmd.PersistentFlags().Bool("include-empty-stacks", false, "Include stacks with no components in the output")

	describeStacksCmd.PersistentFlags().StringSlice("skip", nil, "Skip executing a YAML function in the Atmos stack manifests when executing the command")

	describeCmd.AddCommand(describeStacksCmd)
}
