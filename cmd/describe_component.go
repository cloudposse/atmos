package cmd

import (
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeComponentCmd describes configuration for components
var describeComponentCmd = &cobra.Command{
	Use:                "component",
	Short:              "Execute 'describe component' command",
	Long:               `This command shows configuration for an Atmos component in an Atmos stack: atmos describe component <component> -s <stack>`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {

		err := e.ExecuteDescribeComponentCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.CliConfiguration{}, err)
		}
	},
}

func init() {
	describeComponentCmd.DisableFlagParsing = false
	describeComponentCmd.PersistentFlags().StringP("stack", "s", "", "atmos describe component <component> -s <stack>")
	describeComponentCmd.PersistentFlags().StringP("format", "f", "yaml", "The output format: atmos describe component <component> -s <stack> --format=yaml|json ('yaml' is default)")
	describeComponentCmd.PersistentFlags().String("file", "", "Write the result to the file: atmos describe component <component> -s <stack> --file component.yaml")
	describeComponentCmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command: atmos describe component <component> -s <stack> --process-templates=false")

	err := describeComponentCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.LogErrorAndExit(schema.CliConfiguration{}, err)
	}

	describeCmd.AddCommand(describeComponentCmd)
}
