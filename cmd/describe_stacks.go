package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeStacksCmd describes configuration for stacks and components in the stacks
var describeStacksCmd = &cobra.Command{
	Use:                "stacks",
	Short:              "Execute 'describe stacks' command",
	Long:               `This command shows configuration for atmos stacks and components in the stacks: atmos describe stacks [options]`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteDescribeStacksCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(err)
		}
	},
}

func init() {
	describeStacksCmd.DisableFlagParsing = false

	describeStacksCmd.PersistentFlags().String("file", "", "Write the result to file: atmos describe stacks --file=stacks.yaml")

	describeStacksCmd.PersistentFlags().String("format", "yaml", "Specify the output format: atmos describe stacks --format=yaml|json ('yaml' is default)")

	describeStacksCmd.PersistentFlags().StringP("stack", "s", "",
		"Filter by a specific stack: atmos describe stacks -s <stack>\n"+
			"The filter supports names of the top-level stack manifests (including subfolder paths), and 'atmos' stack names (derived from the context vars)",
	)

	describeStacksCmd.PersistentFlags().String("components", "", "Filter by specific 'atmos' components: atmos describe stacks --components=<component1>,<component2>")

	describeStacksCmd.PersistentFlags().String("component-types", "", "Filter by specific component types: atmos describe stacks --component-types=terraform|helmfile. Supported component types: terraform, helmfile")

	describeStacksCmd.PersistentFlags().String("sections", "", "Output only the specified component sections: atmos describe stacks --sections=vars,settings. Available component sections: backend, backend_type, deps, env, inheritance, metadata, remote_state_backend, remote_state_backend_type, settings, vars")

	describeCmd.AddCommand(describeStacksCmd)
}
