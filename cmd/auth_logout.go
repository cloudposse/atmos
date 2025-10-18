package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// authLogoutCmd logs out by removing local credentials.
var authLogoutCmd = &cobra.Command{
	Use:   "logout [identity]",
	Short: "Remove locally cached credentials and session data",
	Long: `Removes cached credentials from the system keyring and local credential files.

This command removes:
  • Credentials stored in system keyring
  • AWS credential files (~/.aws/atmos/<provider>/credentials)
  • AWS config files (~/.aws/atmos/<provider>/config)

Note: This only removes local credentials. Your browser session with the
identity provider (AWS SSO, Okta, etc.) may still be active. To completely
end your session, visit your identity provider's website and sign out.`,

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
	RunE:               executeAuthLogoutCommand,
}

func executeAuthLogoutCommand(cmd *cobra.Command, args []string) error {
	handleHelpRequest(cmd, args)

	// Load atmos config.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrFailedToInitializeAtmosConfig, err)
	}

	defer perf.Track(&atmosConfig, "cmd.executeAuthLogoutCommand")()

	// Create auth manager.
	authManager, err := createAuthManager(&atmosConfig.Auth)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrAuthManager, err)
	}

	// Get flags.
	providerFlag, _ := cmd.Flags().GetString("provider")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	ctx := context.Background()

	// Determine what to logout.
	if providerFlag != "" {
		// Logout specific provider.
		return performProviderLogout(ctx, authManager, providerFlag, dryRun)
	}

	if len(args) > 0 {
		// Logout specific identity.
		identityName := args[0]
		return performIdentityLogout(ctx, authManager, identityName, dryRun)
	}

	// Interactive mode: prompt user to choose.
	return performInteractiveLogout(ctx, authManager, dryRun)
}

// performIdentityLogout removes credentials for a specific identity.
func performIdentityLogout(ctx context.Context, authManager auth.AuthManager, identityName string, dryRun bool) error {
	// Validate identity exists.
	identities := authManager.GetIdentities()
	if _, exists := identities[identityName]; !exists {
		u.PrintfMessageToTUI("**Error:** identity %q not found in configuration\n\n", identityName)
		u.PrintfMessageToTUI("**Available identities:**\n")
		for name := range identities {
			u.PrintfMessageToTUI("  • %s\n", name)
		}
		u.PrintfMessageToTUI("\n")
		return fmt.Errorf("%w: identity %q", errUtils.ErrIdentityNotInConfig, identityName)
	}

	u.PrintfMessageToTUI("\n**Logging out from identity:** %s\n\n", identityName)

	// Get authentication chain to show what will be removed.
	providerName := authManager.GetProviderForIdentity(identityName)

	if dryRun {
		u.PrintfMessageToTUI("**Dry run mode:** No credentials will be removed\n\n")
		u.PrintfMessageToTUI("**Would remove:**\n")
		u.PrintfMessageToTUI("  • Keyring entry: %s\n", identityName)
		if providerName != "" {
			u.PrintfMessageToTUI("  • Keyring entry: %s (provider)\n", providerName)
			// Get actual configured path for this provider.
			basePath := authManager.GetFilesDisplayPath(providerName)
			u.PrintfMessageToTUI("  • Files: %s/%s/\n", basePath, providerName)
		}
		u.PrintfMessageToTUI("\n")
		return nil
	}

	// Perform logout.
	if err := authManager.Logout(ctx, identityName); err != nil {
		// Check if it's a partial logout.
		if errors.Is(err, errUtils.ErrPartialLogout) {
			u.PrintfMessageToTUI("✓ **Logged out with warnings**\n\n")
			u.PrintfMessageToTUI("Some credentials could not be removed:\n")
			u.PrintfMessageToTUI("  %v\n\n", err)
		} else {
			u.PrintfMessageToTUI("✗ **Logout failed**\n\n")
			u.PrintfMessageToTUI("Error: %v\n\n", err)
			return err
		}
	} else {
		u.PrintfMessageToTUI("✓ **Successfully logged out**\n\n")
	}

	// Display browser session warning.
	displayBrowserWarning()

	return nil
}

// performProviderLogout removes credentials for a specific provider.
func performProviderLogout(ctx context.Context, authManager auth.AuthManager, providerName string, dryRun bool) error {
	// Validate provider exists.
	providers := authManager.GetProviders()
	if _, exists := providers[providerName]; !exists {
		u.PrintfMessageToTUI("**Error:** provider %q not found in configuration\n\n", providerName)
		u.PrintfMessageToTUI("**Available providers:**\n")
		for name := range providers {
			u.PrintfMessageToTUI("  • %s\n", name)
		}
		u.PrintfMessageToTUI("\n")
		return fmt.Errorf("%w: provider %q", errUtils.ErrProviderNotInConfig, providerName)
	}

	u.PrintfMessageToTUI("\n**Logging out from provider:** %s\n\n", providerName)

	if dryRun {
		u.PrintfMessageToTUI("**Dry run mode:** No credentials will be removed\n\n")
		u.PrintfMessageToTUI("**Would remove:**\n")
		u.PrintfMessageToTUI("  • All identities using provider %s\n", providerName)
		u.PrintfMessageToTUI("  • Provider keyring entry\n")
		// Get actual configured path for this provider.
		basePath := authManager.GetFilesDisplayPath(providerName)
		u.PrintfMessageToTUI("  • Files: %s/%s/\n", basePath, providerName)
		u.PrintfMessageToTUI("\n")
		return nil
	}

	// Perform logout.
	if err := authManager.LogoutProvider(ctx, providerName); err != nil {
		u.PrintfMessageToTUI("✗ **Provider logout failed**\n\n")
		u.PrintfMessageToTUI("Error: %v\n\n", err)
		return err
	}

	u.PrintfMessageToTUI("✓ **Successfully logged out from provider**\n\n")

	// Display browser session warning.
	displayBrowserWarning()

	return nil
}

// performInteractiveLogout prompts user to choose what to logout.
func performInteractiveLogout(ctx context.Context, authManager auth.AuthManager, dryRun bool) error {
	identities := authManager.GetIdentities()
	providers := authManager.GetProviders()

	if len(identities) == 0 {
		u.PrintfMessageToTUI("**No identities configured** in atmos.yaml\n")
		return nil
	}

	// Build options list.
	type logoutOption struct {
		label  string
		typ    string // "identity", "provider", "all"
		target string
	}

	var options []logoutOption

	// Add identity options.
	for name := range identities {
		options = append(options, logoutOption{
			label:  fmt.Sprintf("Identity: %s", name),
			typ:    "identity",
			target: name,
		})
	}

	// Add provider options.
	for name := range providers {
		options = append(options, logoutOption{
			label:  fmt.Sprintf("Provider: %s (removes all identities)", name),
			typ:    "provider",
			target: name,
		})
	}

	// Add "all" option.
	options = append(options, logoutOption{
		label:  "All identities (complete logout)",
		typ:    "all",
		target: "",
	})

	// Create select options for huh.
	var huhOptions []huh.Option[logoutOption]
	for _, opt := range options {
		huhOptions = append(huhOptions, huh.NewOption(opt.label, opt))
	}

	var selected logoutOption
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[logoutOption]().
				Title("Choose what to logout from:").
				Options(huhOptions...).
				Value(&selected),
		),
	).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		return err
	}

	// Perform the selected logout action.
	switch selected.typ {
	case "identity":
		return performIdentityLogout(ctx, authManager, selected.target, dryRun)
	case "provider":
		return performProviderLogout(ctx, authManager, selected.target, dryRun)
	case "all":
		return performLogoutAll(ctx, authManager, dryRun)
	default:
		return errUtils.ErrInvalidLogoutOption
	}
}

// performLogoutAll removes all credentials.
func performLogoutAll(ctx context.Context, authManager auth.AuthManager, dryRun bool) error {
	u.PrintfMessageToTUI("\n**Logging out from all identities**\n\n")

	if dryRun {
		u.PrintfMessageToTUI("**Dry run mode:** No credentials will be removed\n\n")
		u.PrintfMessageToTUI("**Would remove:**\n")
		u.PrintfMessageToTUI("  • All identity keyring entries\n")
		u.PrintfMessageToTUI("  • All provider keyring entries\n")

		// Show file paths for each provider.
		providers := authManager.GetProviders()
		if len(providers) > 0 {
			u.PrintfMessageToTUI("  • Files:\n")
			for providerName := range providers {
				basePath := authManager.GetFilesDisplayPath(providerName)
				u.PrintfMessageToTUI("    - %s/%s/\n", basePath, providerName)
			}
		}
		u.PrintfMessageToTUI("\n")
		return nil
	}

	// Perform logout.
	if err := authManager.LogoutAll(ctx); err != nil {
		u.PrintfMessageToTUI("✗ **Logout all failed**\n\n")
		u.PrintfMessageToTUI("Error: %v\n\n", err)
		return err
	}

	u.PrintfMessageToTUI("✓ **Successfully logged out from all identities**\n\n")

	// Display browser session warning.
	displayBrowserWarning()

	return nil
}

// displayBrowserWarning shows a warning about browser sessions.
func displayBrowserWarning() {
	u.PrintfMessageToTUI("⚠️  **Note:** This only removes local credentials.\n")
	u.PrintfMessageToTUI("   Your browser session may still be active. Visit your\n")
	u.PrintfMessageToTUI("   identity provider to end your browser session.\n\n")
}

func init() {
	authLogoutCmd.Flags().String("provider", "", "Logout from specific provider")
	authLogoutCmd.Flags().Bool("dry-run", false, "Preview what would be removed without deleting")
	authCmd.AddCommand(authLogoutCmd)
}
