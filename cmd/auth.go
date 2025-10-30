package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	log "github.com/cloudposse/atmos/pkg/logger"
)

const (
	IdentityFlagName        = "identity"
	IdentityFlagSelectValue = "__SELECT__" // Special value when --identity is used without argument.
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
	authCmd.PersistentFlags().StringP(IdentityFlagName, "i", "", "Specify the target identity to assume. Use without value to interactively select.")

	// Set NoOptDefVal to enable optional flag value.
	// When --identity is used without a value, it will receive IdentityFlagSelectValue.
	identityFlag := authCmd.PersistentFlags().Lookup(IdentityFlagName)
	if identityFlag != nil {
		identityFlag.NoOptDefVal = IdentityFlagSelectValue
	}

	// Bind to Viper and env (flags > env > config > defaults).
	if err := viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY"); err != nil {
		log.Trace("Failed to bind identity environment variables", "error", err)
	}
	if err := viper.BindPFlag(IdentityFlagName, authCmd.PersistentFlags().Lookup(IdentityFlagName)); err != nil {
		log.Trace("Failed to bind identity flag", "error", err)
	}

	// Add completion for identity flag.
	AddIdentityCompletion(authCmd)

	RootCmd.AddCommand(authCmd)
}
