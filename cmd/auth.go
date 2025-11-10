package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
)

const (
	IdentityFlagName = "identity"
	// IdentityFlagSelectValue is imported from cfg.IdentityFlagSelectValue.
	IdentityFlagSelectValue = cfg.IdentityFlagSelectValue
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

	// Bind environment variables but NOT the flag itself.
	// BindPFlag creates a two-way binding that can cause Viper's value to override
	// command-line flags during parsing. Instead, commands should read the flag value
	// first, then fall back to Viper if the flag wasn't provided.
	if err := viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY"); err != nil {
		log.Trace("Failed to bind identity environment variables", "error", err)
	}

	// Add completion for identity flag.
	AddIdentityCompletion(authCmd)

	RootCmd.AddCommand(authCmd)
}
