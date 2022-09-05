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
	atlantisGenerateCmd.AddCommand(atlantisGenerateRepoConfigCmd)
}
