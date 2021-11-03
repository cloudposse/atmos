package cmd

import (
	"fmt"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"os"
)

// describeComponentCmd describes configuration for components
var describeConfigCmd = &cobra.Command{
	Use:                "config",
	Short:              "describe config",
	Long:               `This command shows CLI configuration`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteDescribeConfig(cmd, args)
		if err != nil {
			color.Red("%s\n", err)
			fmt.Println()
			os.Exit(1)
		}
	},
}

func init() {
	describeConfigCmd.DisableFlagParsing = false
	describeConfigCmd.PersistentFlags().StringP("format", "f", "json", "'atmos describe config -f json' or 'atmos describe config -f yaml'")

	describeCmd.AddCommand(describeConfigCmd)
}
