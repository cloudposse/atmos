package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// atlantisGenerateRepoConfigCmd generates repository configuration for Atlantis
var atlantisGenerateRepoConfigCmd = &cobra.Command{
	Use:                "repo-config",
	Short:              "Execute 'atlantis generate repo-config`",
	Long:               "This command generates repository configuration for Atlantis",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteAtlantisGenerateRepoConfigCmd(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	atlantisGenerateRepoConfigCmd.DisableFlagParsing = false

	atlantisGenerateRepoConfigCmd.PersistentFlags().String("config-template", "", "atmos atlantis generate repo-config --config-template config-template-1 --project-template project-template-1")
	atlantisGenerateRepoConfigCmd.PersistentFlags().String("project-template", "", "atmos atlantis generate repo-config --config-template config-template-1 --project-template project-template-1")

	err := atlantisGenerateRepoConfigCmd.MarkPersistentFlagRequired("config-template")
	if err != nil {
		u.PrintErrorToStdErrorAndExit(err)
	}

	err = atlantisGenerateRepoConfigCmd.MarkPersistentFlagRequired("project-template")
	if err != nil {
		u.PrintErrorToStdErrorAndExit(err)
	}

	atlantisGenerateCmd.AddCommand(atlantisGenerateRepoConfigCmd)
}
