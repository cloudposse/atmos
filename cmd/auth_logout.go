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
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
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

You can specify the identity to logout using either:
  • Positional argument: atmos auth logout <identity>
  • Flag: atmos auth logout --identity <identity>

Note: This only removes local credentials. Your browser session with the
identity provider (AWS SSO, Okta, etc.) may still be active. To completely
end your session, visit your identity provider's website and sign out.`,

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  identityArgCompletion,
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

	// Get identity from flag or positional argument.
	// Note: "identity" is a persistent flag on parent authCmd, so it's automatically available.
	identityFlag := ""
	if flag := cmd.Flag("identity"); flag != nil {
		identityFlag = flag.Value.String()
	}

	ctx := context.Background()

	// Determine what to logout.
	if providerFlag != "" {
		// Logout specific provider.
		return performProviderLogout(ctx, authManager, providerFlag, dryRun)
	}

	// Support both positional argument and --identity flag for consistency with other auth commands.
	// Positional argument takes precedence if both are provided.
	var identityName string
	if len(args) > 0 {
		identityName = args[0]
	} else if identityFlag != "" {
		identityName = identityFlag
	}

	if identityName != "" {
		// Logout specific identity.
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
		u.PrintfMarkdownToTUI("**Error:** identity %q not found in configuration\n\n", identityName)
		u.PrintfMarkdownToTUI("**Available identities:**\n")
		for name := range identities {
			u.PrintfMessageToTUI("  • %s\n", name)
		}
		u.PrintfMessageToTUI("\n")
		return fmt.Errorf("%w: identity %q", errUtils.ErrIdentityNotInConfig, identityName)
	}

	// Get authentication chain to show what will be removed.
	providerName := authManager.GetProviderForIdentity(identityName)

	if dryRun {
		u.PrintfMarkdownToTUI("**Dry run mode:** No credentials will be removed\n\n")
		u.PrintfMarkdownToTUI("**Would remove:**\n")
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
			u.PrintfMarkdownToTUI("\n%s Logged out **%s** with warnings\n\n", theme.Styles.Checkmark, identityName)
			u.PrintfMessageToTUI("Some credentials could not be removed:\n")
			u.PrintfMessageToTUI("  %v\n\n", err)
		} else {
			u.PrintfMarkdownToTUI("\n%s Failed to log out **%s**\n\n", theme.Styles.XMark, identityName)
			u.PrintfMessageToTUI("Error: %v\n\n", err)
			return err
		}
	} else {
		u.PrintfMarkdownToTUI("\n%s Logged out **%s**\n\n", theme.Styles.Checkmark, identityName)
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
		u.PrintfMarkdownToTUI("**Error:** provider %q not found in configuration\n\n", providerName)
		u.PrintfMarkdownToTUI("**Available providers:**\n")
		for name := range providers {
			u.PrintfMessageToTUI("  • %s\n", name)
		}
		u.PrintfMessageToTUI("\n")
		return fmt.Errorf("%w: provider %q", errUtils.ErrProviderNotInConfig, providerName)
	}

	// Find all identities using this provider.
	identities := authManager.GetIdentities()
	var identitiesForProvider []string
	for identityName := range identities {
		if authManager.GetProviderForIdentity(identityName) == providerName {
			identitiesForProvider = append(identitiesForProvider, identityName)
		}
	}

	if dryRun {
		u.PrintfMarkdownToTUI("**Dry run mode:** No credentials will be removed\n\n")
		u.PrintfMarkdownToTUI("**Would remove:**\n")
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
		// ErrPartialLogout is treated as success (exit 0) with warning.
		if errors.Is(err, errUtils.ErrPartialLogout) {
			u.PrintfMarkdownToTUI("⚠ **Provider logout partially succeeded**\n\n")
			u.PrintfMessageToTUI("Warning: %v\n\n", err)
			displayBrowserWarning()
			return nil
		}

		u.PrintfMessageToTUI("%s Failed to log out provider\n\n", theme.Styles.XMark)
		u.PrintfMessageToTUI("Error: %v\n\n", err)
		return err
	}

	u.PrintfMarkdownToTUI("\n%s Logged out provider **%s** (%d identities)\n\n", theme.Styles.Checkmark, providerName, len(identitiesForProvider))

	// Display browser session warning.
	displayBrowserWarning()

	return nil
}

// performInteractiveLogout prompts user to choose what to logout.
func performInteractiveLogout(ctx context.Context, authManager auth.AuthManager, dryRun bool) error {
	identities := authManager.GetIdentities()
	providers := authManager.GetProviders()

	if len(identities) == 0 {
		u.PrintfMarkdownToTUI("**No identities configured** in atmos.yaml\n")
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

func performLogoutAll(ctx context.Context, authManager auth.AuthManager, dryRun bool) error {
	if dryRun {
		u.PrintfMarkdownToTUI("**Dry run mode:** No credentials will be removed\n\n")
		u.PrintfMarkdownToTUI("**Would remove:**\n")
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
		// ErrPartialLogout is treated as success (exit 0) with warning.
		if errors.Is(err, errUtils.ErrPartialLogout) {
			u.PrintfMarkdownToTUI("⚠ **Logout all partially succeeded**\n\n")
			u.PrintfMessageToTUI("Warning: %v\n\n", err)
			displayBrowserWarning()
			return nil
		}

		u.PrintfMessageToTUI("%s Failed to log out all identities\n\n", theme.Styles.XMark)
		u.PrintfMessageToTUI("Error: %v\n\n", err)
		return err
	}

	identities := authManager.GetIdentities()
	u.PrintfMessageToTUI("\n%s Logged out all %d identities\n\n", theme.Styles.Checkmark, len(identities))

	// Display browser session warning.
	displayBrowserWarning()

	return nil
}

// displayBrowserWarning shows a warning about browser sessions.
func displayBrowserWarning() {
	// Check if warning has been shown before using cache.
	cache, err := cfg.LoadCache()
	if err == nil && cache.BrowserSessionWarningShown {
		// Warning already shown before, skip it.
		return
	}

	// Show the warning.
	u.PrintfMarkdownToTUI("⚠️  **Note:** This only removes local credentials. Your browser session may still be active. Visit your identity provider to end your browser session.\n\n")

	// Mark warning as shown in cache.
	cache.BrowserSessionWarningShown = true
	if err := cfg.SaveCache(cache); err != nil {
		log.Debug("Failed to save browser warning shown flag to cache", "error", err)
	}
}

func init() {
	authLogoutCmd.Flags().String("provider", "", "Logout from specific provider")
	authLogoutCmd.Flags().Bool("dry-run", false, "Preview what would be removed without deleting")
	authCmd.AddCommand(authLogoutCmd)
}
