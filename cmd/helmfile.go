package cmd

import (
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// terraformCmd represents the base command for all terraform sub-commands
var helmfileCmd = &cobra.Command{
	Use:                "helmfile",
	Short:              "helmfile command",
	Long:               `This command runs helmfile sub-commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteHelmfile(cmd, args)
		if err != nil {
			color.Red("%s\n\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	//helmfileCmd.DisableFlagParsing = true  // This breaks the help for this command
	helmfileCmd.PersistentFlags().StringP("stack", "s", "", "")

	err := helmfileCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		color.Red("%s\n\n", err)
		os.Exit(1)
	}

	RootCmd.AddCommand(helmfileCmd)
}
