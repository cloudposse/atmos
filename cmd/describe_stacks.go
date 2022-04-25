package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/spf13/cobra"
	"os"
)

// describeStacksCmd describes configuration for stacks and components in the stacks
var describeStacksCmd = &cobra.Command{
	Use:                "stacks",
	Short:              "Execute 'describe stacks' command",
	Long:               `This command shows configuration for stacks and components in the stacks: atmos describe stacks <options>`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteDescribeStacks(cmd, args)
		if err != nil {
			os.Exit(1)
		}
	},
}

func init() {
	describeStacksCmd.DisableFlagParsing = false
	describeStacksCmd.PersistentFlags().String("file", "", "Write the result to file: atmos describe stacks --file=stacks.yaml")
	describeStacksCmd.PersistentFlags().String("format", "yaml", "Specify output format: atmos describe stacks --format=yaml/json ('yaml' is default)")
	describeStacksCmd.PersistentFlags().StringP("stack", "s", "", "Filter by a specific stack: atmos describe stacks -s <stack>")
	describeStacksCmd.PersistentFlags().String("components", "", "Filter by specific components: atmos describe stacks --components=<component1>,<component2>")
	describeStacksCmd.PersistentFlags().String("component-types", "", "Filter by specific component types: atmos describe stacks --component-types=terraform,helmfile. Supported component types: terraform, helmfile")
	describeStacksCmd.PersistentFlags().String("sections", "", "Output only these component sections: atmos describe stacks --sections=vars,settings. Available component sections: backend, backend_type, deps, env, inheritance, metadata, remote_state_backend, remote_state_backend_type, settings, vars")

	describeCmd.AddCommand(describeStacksCmd)
}
