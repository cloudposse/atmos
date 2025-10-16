package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	log "github.com/cloudposse/atmos/pkg/logger"
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
	if err := viper.BindEnv(ProfileFlagName, "ATMOS_PROFILE", "PROFILE"); err != nil {
		log.Trace("Failed to bind profile environment variables", "error", err)
	}
	if err := viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY"); err != nil {
		log.Trace("Failed to bind identity environment variables", "error", err)
	}
	if err := viper.BindPFlag(ProfileFlagName, authCmd.PersistentFlags().Lookup(ProfileFlagName)); err != nil {
		log.Trace("Failed to bind profile flag", "error", err)
	}
	if err := viper.BindPFlag(IdentityFlagName, authCmd.PersistentFlags().Lookup(IdentityFlagName)); err != nil {
		log.Trace("Failed to bind identity flag", "error", err)
	}

	// Add completion for identity flag.
	AddIdentityCompletion(authCmd)

	RootCmd.AddCommand(authCmd)
}
