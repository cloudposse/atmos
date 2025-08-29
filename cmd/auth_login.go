package cmd

import (
	"github.com/cloudposse/atmos/internal/auth"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// authLoginCmd logs in using a configured identity.
var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate using a configured identity",
	Long:  "Authenticate to cloud providers using an identity defined in atmos.yaml.",

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
	Run: func(cmd *cobra.Command, args []string) {
		err := auth.ExecuteAuthLoginCommand(cmd, args)
		if err != nil {
			u.PrintfMarkdown("%v", err)
		}
	},
}

func init() {
	AddStackCompletion(authLoginCmd)
	authCmd.AddCommand(authLoginCmd)
}
