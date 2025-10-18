package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// authLoginCmd logs in using a configured identity.
var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate using a configured identity",
	Long:  "Authenticate to cloud providers using an identity defined in `atmos.yaml`.",

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
	RunE:               executeAuthLoginCommand,
}

func executeAuthLoginCommand(cmd *cobra.Command, args []string) error {
	handleHelpRequest(cmd, args)

	// Load atmos config.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToInitConfig, err)
	}
	defer perf.Track(&atmosConfig, "cmd.executeAuthLoginCommand")()

	// Create auth manager.
	authManager, err := createAuthManager(&atmosConfig.Auth)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Get identity from flag or use default.
	identityName := viper.GetString(IdentityFlagName)

	// If no identity specified, get the default identity (which prompts if needed).
	if identityName == "" {
		identityName, err = authManager.GetDefaultIdentity()
		if err != nil {
			return errors.Join(errUtils.ErrDefaultIdentity, err)
		}
	}

	// Perform authentication.
	ctx := context.Background()
	whoami, err := authManager.Authenticate(ctx, identityName)
	if err != nil {
		return errors.Join(errUtils.ErrAuthenticationFailed, fmt.Errorf("identity=%s: %w", identityName, err))
	}

	// Display success message.
	u.PrintfMessageToTUI("**Authentication successful!**\n")
	u.PrintfMessageToTUI("Provider: %s\n", whoami.Provider)
	u.PrintfMessageToTUI("Identity: %s\n", whoami.Identity)
	if whoami.Account != "" {
		u.PrintfMessageToTUI("Account: %s\n", whoami.Account)
	}
	if whoami.Region != "" {
		u.PrintfMessageToTUI("Region: %s\n", whoami.Region)
	}
	if whoami.Expiration != nil {
		u.PrintfMessageToTUI("Expires: %s\n", whoami.Expiration.Format("2006-01-02 15:04:05 MST"))
	}

	return nil
}

// createAuthManager creates a new auth manager with all required dependencies.
func createAuthManager(authConfig *schema.AuthConfig) (auth.AuthManager, error) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	return auth.NewAuthManager(authConfig, credStore, validator, nil)
}

func init() {
	authCmd.AddCommand(authLoginCmd)
}
