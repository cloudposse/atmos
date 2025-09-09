package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	ProfileFlagName  = "profile"
	IdentityFlagName = "identity"
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
	authCmd.PersistentFlags().String(ProfileFlagName, "", "Specify the profile to use for authentication.")
	authCmd.PersistentFlags().StringP(IdentityFlagName, "i", "", "Specify the identity to authenticate to.")
	// Bind to Viper and env.
	viper.MustBindEnv(ProfileFlagName, ProfileFlagName, "ATMOS_PROFILE")
	viper.MustBindEnv(IdentityFlagName, IdentityFlagName, "ATMOS_IDENTITY")
	_ = viper.BindPFlag(ProfileFlagName, authCmd.PersistentFlags().Lookup(ProfileFlagName))
	_ = viper.BindPFlag(IdentityFlagName, authCmd.PersistentFlags().Lookup(IdentityFlagName))

	RootCmd.AddCommand(authCmd)
}
