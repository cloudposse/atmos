package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeComponentCmd describes configuration for components
var describeConfigCmd = &cobra.Command{
	Use:                "config",
	Short:              "Display the final merged CLI configuration",
	Long:               "This command displays the final, deep-merged CLI configuration after combining all relevant configuration files.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteDescribeConfigCmd(cmd, args)
		if err != nil {
			u.PrintErrorMarkdown("", err, "")
		}
	},
}

func init() {
	describeConfigCmd.DisableFlagParsing = false
	describeConfigCmd.PersistentFlags().StringP("format", "f", "json", "The output format")

	describeCmd.AddCommand(describeConfigCmd)
}
