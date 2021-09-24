package cmd

import (
	e "atmos/internal/exec"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"os"
)

// describeComponentCmd describes configuration for components
var describeComponentCmd = &cobra.Command{
	Use:                "component",
	Short:              "describe component",
	Long:               `This command shows configuration for components`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteDescribeComponent(cmd, args)
		if err != nil {
			color.Red("%s\n\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	describeComponentCmd.DisableFlagParsing = false
	describeComponentCmd.PersistentFlags().StringP("stack", "s", "", "")

	err := describeComponentCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		color.Red("%s\n\n", err)
		os.Exit(1)
	}

	describeCmd.AddCommand(describeComponentCmd)
}
