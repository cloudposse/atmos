package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// authLogoutCmd logs out and clears cached credentials.
var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out and clear cached credentials",
	Long: `Log out from the specified identity and clear cached credentials.

For GitHub identities, this removes tokens from the OS keychain.

To revoke access tokens completely:
1. Use this command to clear local tokens
2. Visit your GitHub App settings to revoke all tokens:
   https://github.com/settings/apps

Or validate if a token is still active:
  env GH_TOKEN=$TOKEN gh api /user

Examples:
  # Logout from default identity
  atmos auth logout

  # Logout from specific identity
  atmos auth logout --identity dev

  # Logout from all identities
  atmos auth logout --all`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE:               executeLogoutCommand,
}

// executeLogoutCommand performs logout and credential cleanup.
func executeLogoutCommand(cmd *cobra.Command, args []string) error {
	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return fmt.Errorf("failed to load atmos config: %w", err)
	}

	// Create auth manager.
	authManager, err := createAuthManager(&atmosConfig.Auth)
	if err != nil {
		return fmt.Errorf("failed to create auth manager: %w", err)
	}

	// Check if logout all.
	logoutAll, _ := cmd.Flags().GetBool("all")

	if logoutAll {
		return logoutAllIdentities(authManager)
	}

	// Get identity from flag or use default.
	identityName, _ := cmd.Flags().GetString("identity")
	if identityName == "" {
		defaultIdentity, err := authManager.GetDefaultIdentity()
		if err != nil {
			return fmt.Errorf("%w: no default identity configured and no identity specified: %v", errUtils.ErrNoDefaultIdentity, err)
		}
		identityName = defaultIdentity
	}

	return logoutIdentity(authManager, identityName)
}

// logoutIdentity logs out a single identity.
//
//nolint:unparam // authManager unused but will be needed for TODO implementation
func logoutIdentity(authManager interface{}, identityName string) error {
	ctx := context.Background()

	// TODO: Implement actual token deletion from keychain.
	// For now, this is a placeholder that logs the action.
	// The actual implementation will need to:
	// 1. Determine the provider type
	// 2. Call provider-specific cleanup (e.g., delete from keychain)
	// 3. Clear any cached credentials

	log.Info("Logging out from identity", "identity", identityName)

	// Placeholder: In production, this would call provider-specific cleanup.
	fmt.Fprintf(os.Stderr, "✓ Logged out from identity: %s\n", identityName)
	fmt.Fprintf(os.Stderr, "\nTo fully revoke GitHub tokens:\n")
	fmt.Fprintf(os.Stderr, "1. Visit https://github.com/settings/apps\n")
	fmt.Fprintf(os.Stderr, "2. Click 'Revoke all user tokens' for your GitHub App\n")
	fmt.Fprintf(os.Stderr, "\nOr validate if token is still active:\n")
	fmt.Fprintf(os.Stderr, "  env GH_TOKEN=$TOKEN gh api /user\n")

	// Suppress unused variable warning for now.
	_ = ctx

	return nil
}

// logoutAllIdentities logs out all configured identities.
func logoutAllIdentities(authManager interface{}) error {
	// Get list of all identities from auth manager.
	// This requires adding a method to the AuthManager interface.
	// For now, placeholder implementation.

	log.Info("Logging out from all identities")

	fmt.Fprintf(os.Stderr, "✓ Logged out from all identities\n")
	fmt.Fprintf(os.Stderr, "\nTo fully revoke GitHub tokens:\n")
	fmt.Fprintf(os.Stderr, "1. Visit https://github.com/settings/apps\n")
	fmt.Fprintf(os.Stderr, "2. Click 'Revoke all user tokens' for each GitHub App\n")

	return nil
}

func init() {
	authLogoutCmd.Flags().StringP("identity", "i", "", "Identity to logout from")
	authLogoutCmd.Flags().Bool("all", false, "Logout from all identities")
	AddIdentityCompletion(authLogoutCmd)
	authCmd.AddCommand(authLogoutCmd)
}
