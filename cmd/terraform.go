package cmd

import (
	e "atmos/internal/exec"
	"github.com/spf13/cobra"
	"log"
)

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:                "terraform",
	Short:              "Terraform command",
	Long:               `This command runs terraform sub-commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteTerraform(cmd, args)
		if err != nil {
			log.Fatalln(err)
		}
	},
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	terraformCmd.DisableFlagParsing = true
	terraformCmd.PersistentFlags().StringP("stack", "s", "", "")

	err := terraformCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		log.Fatalln(err)
		return
	}

	RootCmd.AddCommand(terraformCmd)
}
