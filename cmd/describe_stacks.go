package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// describeComponentCmd describes configuration for components
var describeStacksCmd = &cobra.Command{
	Use:                "stacks",
	Short:              "Execute 'describe stacks' command",
	Long:               `This command shows configuration for stacks and components in the stacks: atmos describe stacks`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteDescribeStacks(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	describeStacksCmd.DisableFlagParsing = false
	describeStacksCmd.PersistentFlags().String("format", "yaml", "atmos describe stacks --format=yaml/json ('yaml' is default)")
	describeStacksCmd.PersistentFlags().StringP("file", "f", "", "Write the result to file: atmos describe stacks --file=stacks.yaml")
	describeStacksCmd.PersistentFlags().String("components", "", "Filter by components: atmos describe stacks --components=<component1>,<component2>")
	describeStacksCmd.PersistentFlags().String("sections", "", "Output only these sections: atmos describe stacks --sections=vars,settings. Available sections: backend, backend_type, deps, env, inheritance, metadata, remote_state_backend, remote_state_backend_type, settings, vars")

	describeCmd.AddCommand(describeStacksCmd)
}
