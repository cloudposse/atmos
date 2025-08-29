package cmd

import (
	"github.com/spf13/cobra"
)

// authCmd groups authentication-related subcommands.
var authCmd = &cobra.Command{
	Use:                "auth",
	Short:              "Authentication commands",
	Long:               "Commands to authenticate and manage identities for Atmos.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
}

func init() {
	AddStackCompletion(authCmd)
	authCmd.PersistentFlags().String("profile", "", "Specify the profile to use for authentication")
	authCmd.PersistentFlags().StringP("identity", "i", "", "Specify the identity to authenticate to.")
	RootCmd.AddCommand(authCmd)
}
