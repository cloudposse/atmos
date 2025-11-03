package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GetIdentityFromFlags retrieves the identity value from command-line flags and environment variables.
// This function handles the Cobra NoOptDefVal quirk where --identity <value> with positional args
// can be misinterpreted as --identity (without value).
//
// Returns:
//   - identity value if explicitly provided
//   - cfg.IdentityFlagSelectValue if --identity was used without a value (interactive selection)
//   - value from ATMOS_IDENTITY env var if flag not provided
//   - empty string if no identity specified anywhere
//
// Usage:
//
//	identity := GetIdentityFromFlags(cmd, os.Args)
//	if identity == cfg.IdentityFlagSelectValue {
//	    // Show interactive selector
//	} else if identity != "" {
//	    // Use explicit identity
//	}
func GetIdentityFromFlags(cmd *cobra.Command, osArgs []string) string {
	// Check if flag was set via Cobra first (handles both SetArgs in tests and real CLI).
	// For commands without positional args, this is reliable.
	// For commands with positional args, we'll use os.Args parsing as a fallback.
	if cmd.Flags().Changed(IdentityFlagName) {
		value, _ := cmd.Flags().GetString(IdentityFlagName)
		// Only trust this value if it's not the NoOptDefVal issue.
		// If we got "__SELECT__" but there might be a real value in os.Args, check os.Args.
		if value != cfg.IdentityFlagSelectValue {
			return value
		}
		// Got __SELECT__ - check if os.Args has an actual value.
		identity := extractIdentityFromArgs(osArgs)
		if identity != "" && identity != cfg.IdentityFlagSelectValue {
			// Found actual value in os.Args - use that instead.
			return identity
		}
		// No value in os.Args either - return __SELECT__.
		return value
	}

	// Flag not changed - fall back to environment variable.
	return viper.GetString(IdentityFlagName)
}

// extractIdentityFromArgs manually parses os.Args to find --identity flag and its value.
// This is necessary because Cobra's NoOptDefVal behavior causes it to misinterpret
// "--identity value" as "--identity" (without value) when positional args are present.
//
// Handles three cases:
//   - --identity value (space-separated) -> returns "value"
//   - --identity=value (equals sign) -> returns "value"
//   - --identity (no value) -> returns cfg.IdentityFlagSelectValue
//
// Returns empty string if --identity flag is not present in args.
func extractIdentityFromArgs(args []string) string {
	for i, arg := range args {
		// Handle --identity=value format.
		if strings.HasPrefix(arg, cfg.IdentityFlag+"=") {
			value := strings.TrimPrefix(arg, cfg.IdentityFlag+"=")
			if value == "" {
				// --identity= (empty value) -> interactive selection.
				return cfg.IdentityFlagSelectValue
			}
			return value
		}

		// Handle -i=value format (short flag).
		if strings.HasPrefix(arg, "-i=") {
			value := strings.TrimPrefix(arg, "-i=")
			if value == "" {
				// -i= (empty value) -> interactive selection.
				return cfg.IdentityFlagSelectValue
			}
			return value
		}

		// Handle --identity value format (space-separated).
		if arg == cfg.IdentityFlag || arg == "-i" {
			// Check if next arg exists and is not another flag.
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				// Has value: --identity <value>.
				return args[i+1]
			}
			// No value: --identity (interactive selection).
			return cfg.IdentityFlagSelectValue
		}
	}

	// Flag not found in args.
	return ""
}

// CreateAuthManagerFromIdentity creates and authenticates an AuthManager from an identity name.
// Returns nil if identityName is empty (no authentication requested).
// Returns error if identityName is provided but auth is not configured in atmos.yaml.
// This helper reduces nested complexity in describe commands.
func CreateAuthManagerFromIdentity(
	identityName string,
	authConfig *schema.AuthConfig,
) (auth.AuthManager, error) {
	if identityName == "" {
		return nil, nil
	}

	// Check if auth is configured when --identity flag is provided.
	if authConfig == nil || len(authConfig.Identities) == 0 {
		return nil, fmt.Errorf("%w: the --identity flag requires authentication to be configured in atmos.yaml with at least one identity", errUtils.ErrAuthNotConfigured)
	}

	// Create a ConfigAndStacksInfo for the auth manager to populate with AuthContext.
	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}

	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()
	authManager, err := auth.NewAuthManager(authConfig, credStore, validator, authStackInfo)
	if err != nil {
		return nil, errors.Join(errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Handle interactive selection.
	forceSelect := identityName == IdentityFlagSelectValue
	if forceSelect {
		identityName, err = authManager.GetDefaultIdentity(forceSelect)
		if err != nil {
			return nil, err
		}
	}

	// Authenticate.
	_, err = authManager.Authenticate(context.Background(), identityName)
	if err != nil {
		return nil, err
	}

	return authManager, nil
}
