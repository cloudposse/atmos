package cmd

import (
	"github.com/spf13/cobra"
)

// authCmd executes 'aws sso login' command
var authCmd = &cobra.Command{
	Use:                "auth",
	Short:              "",
	Long:               ``,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
}

func init() {
	AddStackCompletion(authCmd)
	authCmd.PersistentFlags().String("profile", "", "Specify the profile to use for authentication")
	authCmd.PersistentFlags().StringP("identity", "i", "", "Specify the identity to authenticate to.")
	RootCmd.AddCommand(authCmd)
}
