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
	Short:              "Authenticate with cloud providers and identity services.",
	Long:               "Obtain, refresh, and configure credentials from external identity providers such as AWS SSO, Vault, or OIDC. Provides the necessary authentication context for tools like Terraform and Helm to interact with cloud infrastructure.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
}

func init() {
	// Avoid adding "stack" at the group level unless subcommands require it.
	// AddStackCompletion(authCmd)
	authCmd.PersistentFlags().String(ProfileFlagName, "", "Specify the profile to use for authentication.")
	authCmd.PersistentFlags().StringP(IdentityFlagName, "i", "", "Specify the target identity to assume.")
	// Bind to Viper and env (flags > env > config > defaults).
	_ = viper.BindEnv(ProfileFlagName, "ATMOS_PROFILE", "PROFILE")
	_ = viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY")
	_ = viper.BindPFlag(ProfileFlagName, authCmd.PersistentFlags().Lookup(ProfileFlagName))
	_ = viper.BindPFlag(IdentityFlagName, authCmd.PersistentFlags().Lookup(IdentityFlagName))

	RootCmd.AddCommand(authCmd)
}
