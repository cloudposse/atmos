package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// loginParser handles flags for the login command.
var loginParser *flags.StandardParser

// authLoginCmd logs in using a configured identity.
var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate using a configured identity",
	Long:  "Authenticate to cloud providers using an identity defined in `atmos.yaml`.",

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
	RunE:               executeAuthLoginCommand,
}

func init() {
	defer perf.Track(nil, "auth.login.init")()

	// Create parser with login-specific flags.
	loginParser = flags.NewStandardParser(
		flags.WithStringFlag("provider", "p", "", "Provider name to authenticate with (for SSO auto-provisioning)"),
	)

	// Register flags with the command.
	loginParser.RegisterFlags(authLoginCmd)

	// Bind to Viper for environment variable support.
	if err := loginParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add to parent command.
	authCmd.AddCommand(authLoginCmd)
}

func executeAuthLoginCommand(cmd *cobra.Command, args []string) error {
	handleHelpRequest(cmd, args)

	// Bind parsed flags to Viper for precedence (must be done before parsing global flags).
	v := viper.GetViper()
	if err := loginParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Parse global flags and build ConfigAndStacksInfo to honor --base-path, --config, --config-path, --profile.
	configAndStacksInfo := BuildConfigAndStacksInfo(cmd, v)

	// Load atmos config.
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitConfig, err)
	}
	defer perf.Track(&atmosConfig, "auth.executeAuthLoginCommand")()

	// Create auth manager.
	authManager, err := CreateAuthManager(&atmosConfig.Auth)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Check if --provider flag was provided.
	providerName := v.GetString("provider")

	// Perform authentication based on whether provider or identity was specified.
	ctx := context.Background()
	var whoami *authTypes.WhoamiInfo

	if providerName != "" {
		// Provider-level authentication (e.g., for SSO auto-provisioning).
		whoami, err = authManager.AuthenticateProvider(ctx, providerName)
		if err != nil {
			return fmt.Errorf("%w: provider=%s: %w", errUtils.ErrAuthenticationFailed, providerName, err)
		}
	} else {
		// Identity-level authentication (existing behavior).
		whoami, err = authenticateIdentity(ctx, cmd, authManager)
		if err != nil {
			return err
		}
	}

	// Display success message using Atmos theme.
	displayAuthSuccess(whoami)

	return nil
}

// authenticateIdentity handles identity-level authentication with default and interactive selection.
func authenticateIdentity(ctx context.Context, cmd *cobra.Command, authManager auth.AuthManager) (*authTypes.WhoamiInfo, error) {
	defer perf.Track(nil, "auth.authenticateIdentity")()

	// Get identity from flag or use default.
	// Use centralized function that handles Cobra's NoOptDefVal quirk correctly.
	identityName := GetIdentityFromFlags(cmd)

	// If flag wasn't provided, check Viper for env var fallback.
	if identityName == "" {
		identityName = viper.GetString(IdentityFlagName)
	}

	// Check if user wants to interactively select identity.
	forceSelect := identityName == IdentityFlagSelectValue

	// If no identity specified, get the default identity (which prompts if needed).
	// If --identity flag was used without value, forceSelect will be true.
	if identityName == "" || forceSelect {
		var err error
		identityName, err = authManager.GetDefaultIdentity(forceSelect)
		if err != nil {
			return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrDefaultIdentity, err)
		}
	}

	// Perform identity authentication.
	whoami, err := authManager.Authenticate(ctx, identityName)
	if err != nil {
		return nil, fmt.Errorf("%w: identity=%s: %w", errUtils.ErrAuthenticationFailed, identityName, err)
	}

	return whoami, nil
}
