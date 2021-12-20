package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"os"
)

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:                "terraform",
	Short:              "Execute 'terraform' commands",
	Long:               `This command runs terraform sub-commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteTerraform(cmd, args)
		if err != nil {
			color.Red("%s\n\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	terraformCmd.DisableFlagParsing = true
	terraformCmd.PersistentFlags().StringP("stack", "s", "", "")

	err := terraformCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		color.Red("%s\n\n", err)
		os.Exit(1)
	}

	RootCmd.AddCommand(terraformCmd)
}
