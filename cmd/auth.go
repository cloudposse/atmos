package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	// Avoid adding "stack" at the group level unless subcommands require it.
	// AddStackCompletion(authCmd)
	authCmd.PersistentFlags().String("profile", "", "Specify the profile to use for authentication.")
	authCmd.PersistentFlags().StringP("identity", "i", "", "Specify the identity to authenticate to.")
	// Bind to Viper and env.
	viper.MustBindEnv("profile", "PROFILE", "ATMOS_PROFILE")
	viper.MustBindEnv("identity", "IDENTITY", "ATMOS_IDENTITY")
	_ = viper.BindPFlag("profile", authCmd.PersistentFlags().Lookup("profile"))
	_ = viper.BindPFlag("identity", authCmd.PersistentFlags().Lookup("identity"))

	RootCmd.AddCommand(authCmd)
}
