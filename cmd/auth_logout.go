package cmd

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

//go:embed markdown/atmos_auth_logout_usage.md
var authLogoutUsageMarkdown string

// authLogoutCmd logs out by removing local credentials.
var authLogoutCmd = &cobra.Command{
	Use:   "logout [identity]",
	Short: "End session by clearing session data",
	Long: `Ends your session by clearing session data (tokens, cached credentials).

By default, preserves keychain credentials (access keys, service account credentials) for instant
re-authentication. Only session data is cleared (SSO tokens, temporary credentials, session files).

Use --keychain to also delete keychain credentials (destructive, requires confirmation).

Note: This only removes local credentials. Your browser session with your identity provider
may still be active. Works with all cloud providers (AWS, Azure, GCP, etc.).`,

	Example:            authLogoutUsageMarkdown,
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
	allFlag, _ := cmd.Flags().GetBool("all")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	deleteKeychain, _ := cmd.Flags().GetBool("keychain")
	force, _ := cmd.Flags().GetBool("force")

	// Get identity from flag or positional argument.
	// Note: "identity" is a persistent flag on parent authCmd, so it's automatically available.
	identityFlag := ""
	if flag := cmd.Flag("identity"); flag != nil {
		identityFlag = flag.Value.String()
	}

	ctx := context.Background()

	// Determine what to logout.
	if allFlag {
		// Logout all identities.
		return performLogoutAll(ctx, authManager, dryRun, deleteKeychain, force)
	}

	if providerFlag != "" {
		// Logout specific provider.
		return performProviderLogout(ctx, authManager, providerFlag, dryRun, deleteKeychain, force)
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
		return performIdentityLogout(ctx, authManager, identityName, dryRun, deleteKeychain, force)
	}

	// Interactive mode: prompt user to choose.
	return performInteractiveLogout(ctx, authManager, dryRun, deleteKeychain, force)
}

// buildKeychainDeletionMessage creates the confirmation message for keychain deletion.
// This is a pure function that can be easily tested.
func buildKeychainDeletionMessage(identityOrProvider string) string {
	return fmt.Sprintf(
		"Delete keychain credentials for %s?\n\n"+
			"This will permanently remove:\n"+
			"  • Access keys and credentials\n"+
			"  • Service account credentials\n"+
			"  • Provider credentials\n\n"+
			"Session data will also be cleared.",
		identityOrProvider,
	)
}

// confirmKeychainDeletion shows interactive Huh confirmation when --keychain is used in TTY.
// Returns true if user confirms, false if cancelled or in non-TTY without --force.
func confirmKeychainDeletion(identityOrProvider string, force bool, isTTY bool) (bool, error) {
	// If force flag is set, skip confirmation.
	if force {
		return true, nil
	}

	// If not a TTY, show error and require --force.
	if !isTTY {
		u.PrintfMarkdownToTUI("⚠ **Warning:** `--keychain` specified but not in interactive terminal\n\n")
		u.PrintfMarkdownToTUI("Keychain deletion requires confirmation. Options:\n")
		u.PrintfMarkdownToTUI("  • Use `--force` to bypass confirmation in CI/CD\n")
		u.PrintfMarkdownToTUI("  • Run interactively to confirm deletion\n\n")
		u.PrintfMarkdownToTUI("%s Logout cancelled - use `--force` to delete keychain in non-interactive mode\n\n", theme.Styles.XMark)
		return false, errUtils.ErrKeychainDeletionRequiresConfirmation
	}

	// Build prompt message.
	message := buildKeychainDeletionMessage(identityOrProvider)

	var confirmed bool

	// Create Huh confirmation prompt.
	confirmPrompt := huh.NewConfirm().
		Title(message).
		Affirmative("Yes, delete credentials").
		Negative("No, keep credentials").
		Value(&confirmed).
		WithTheme(uiutils.NewAtmosHuhTheme())

	if err := confirmPrompt.Run(); err != nil {
		return false, fmt.Errorf("confirmation prompt failed: %w", err)
	}

	if !confirmed {
		u.PrintfMessageToTUI("\nLogout cancelled - credentials preserved\n\n")
		return false, nil
	}

	return true, nil
}

// detectExternalCredentials checks for environment variables pointing to external credential files.
// Returns a list of warnings about external credentials that may still be active.
// Note: These are external provider-specific env vars (GCP, Azure, AWS), not Atmos configuration.
// They are checked read-only to detect if external credentials are still in use.
func detectExternalCredentials() []string {
	var warnings []string

	//nolint:forbidigo // Check for external provider env vars that may still be active.
	// Check for Google ADC.
	if gcpCreds := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); gcpCreds != "" {
		warnings = append(warnings, fmt.Sprintf("GOOGLE_APPLICATION_CREDENTIALS=%s", gcpCreds))
	}

	// Check for Azure service principal.
	//nolint:forbidigo
	if azureCertPath := os.Getenv("AZURE_CERTIFICATE_PATH"); azureCertPath != "" {
		warnings = append(warnings, fmt.Sprintf("AZURE_CERTIFICATE_PATH=%s", azureCertPath))
	}

	// Check for AWS shared credentials file override.
	//nolint:forbidigo
	if awsCredsFile := os.Getenv("AWS_SHARED_CREDENTIALS_FILE"); awsCredsFile != "" {
		warnings = append(warnings, fmt.Sprintf("AWS_SHARED_CREDENTIALS_FILE=%s", awsCredsFile))
	}

	return warnings
}

// displayExternalCredentialWarnings shows warnings about external credentials.
func displayExternalCredentialWarnings() {
	warnings := detectExternalCredentials()

	if len(warnings) == 0 {
		return
	}

	u.PrintfMarkdownToTUI("\n⚠ **Warning:** External credentials may still be active:\n\n")
	for _, warning := range warnings {
		u.PrintfMessageToTUI("  • %s\n", warning)
	}
	u.PrintfMarkdownToTUI("\nTo fully logout, consider:\n")
	u.PrintfMarkdownToTUI("  • Unsetting the environment variable\n")
	u.PrintfMarkdownToTUI("  • Removing the credentials file\n\n")
}

// performIdentityLogout removes credentials for a specific identity.
func performIdentityLogout(ctx context.Context, authManager auth.AuthManager, identityName string, dryRun bool, deleteKeychain bool, force bool) error { //nolint:gocognit,revive
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
		if deleteKeychain {
			u.PrintfMessageToTUI("  • Keyring entry: %s\n", identityName)
			if providerName != "" {
				u.PrintfMessageToTUI("  • Keyring entry: %s (provider)\n", providerName)
			}
		}
		if providerName != "" {
			// Get actual configured path for this provider.
			basePath := authManager.GetFilesDisplayPath(providerName)
			u.PrintfMessageToTUI("  • Files: %s/%s/\n", basePath, providerName)
		}
		u.PrintfMessageToTUI("\n")
		return nil
	}

	// If deleteKeychain is requested, confirm with user.
	if deleteKeychain {
		isTTY := term.IsTTYSupportForStdout()
		confirmed, err := confirmKeychainDeletion(identityName, force, isTTY)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil // User cancelled, exit cleanly.
		}
	}

	// Perform logout.
	if err := authManager.Logout(ctx, identityName, deleteKeychain); err != nil {
		// Check if it's a partial logout.
		if errors.Is(err, errUtils.ErrPartialLogout) {
			u.PrintfMessageToTUI("\n%s Logged out %s with warnings\n\n", theme.Styles.Checkmark, identityName)
			u.PrintfMessageToTUI("Some credentials could not be removed:\n")
			u.PrintfMessageToTUI("  %v\n\n", err)
		} else {
			u.PrintfMessageToTUI("\n%s Failed to log out %s\n\n", theme.Styles.XMark, identityName)
			u.PrintfMessageToTUI("Error: %v\n\n", err)
			return err
		}
	} else {
		u.PrintfMessageToTUI("\n%s Logged out %s\n\n", theme.Styles.Checkmark, identityName)
	}

	// Display browser session warning.
	displayBrowserWarning()

	// Display external credential warnings.
	displayExternalCredentialWarnings()

	return nil
}

// performProviderLogout removes credentials for a specific provider.
func performProviderLogout(ctx context.Context, authManager auth.AuthManager, providerName string, dryRun bool, deleteKeychain bool, force bool) error { //nolint:funlen,revive
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
		if deleteKeychain {
			u.PrintfMessageToTUI("  • Provider keyring entry\n")
		}
		// Get actual configured path for this provider.
		basePath := authManager.GetFilesDisplayPath(providerName)
		u.PrintfMessageToTUI("  • Files: %s/%s/\n", basePath, providerName)
		u.PrintfMessageToTUI("\n")
		return nil
	}

	// If deleteKeychain is requested, confirm with user.
	if deleteKeychain {
		isTTY := term.IsTTYSupportForStdout()
		confirmed, err := confirmKeychainDeletion(providerName, force, isTTY)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil // User cancelled, exit cleanly.
		}
	}

	// Perform logout.
	if err := authManager.LogoutProvider(ctx, providerName, deleteKeychain); err != nil {
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

	u.PrintfMessageToTUI("\n%s Logged out provider %s (%d identities)\n\n", theme.Styles.Checkmark, providerName, len(identitiesForProvider))

	// Display browser session warning.
	displayBrowserWarning()

	// Display external credential warnings.
	displayExternalCredentialWarnings()

	return nil
}

// logoutOption represents a logout choice in the interactive menu.
type logoutOption struct {
	label  string
	typ    string // "identity", "provider", "all"
	target string
}

// buildLogoutOptions creates the list of logout options from identities and providers.
// This is a pure function that can be easily tested.
func buildLogoutOptions(identities map[string]schema.Identity, providers map[string]schema.Provider) []logoutOption {
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

	return options
}

// executeLogoutOption executes the selected logout action.
// This is a pure routing function that can be easily tested.
func executeLogoutOption(ctx context.Context, authManager auth.AuthManager, option logoutOption, dryRun bool, deleteKeychain bool, force bool) error { //nolint:revive
	switch option.typ {
	case "identity":
		return performIdentityLogout(ctx, authManager, option.target, dryRun, deleteKeychain, force)
	case "provider":
		return performProviderLogout(ctx, authManager, option.target, dryRun, deleteKeychain, force)
	case "all":
		return performLogoutAll(ctx, authManager, dryRun, deleteKeychain, force)
	default:
		return errUtils.ErrInvalidLogoutOption
	}
}

// performInteractiveLogout prompts user to choose what to logout.
func performInteractiveLogout(ctx context.Context, authManager auth.AuthManager, dryRun bool, deleteKeychain bool, force bool) error {
	identities := authManager.GetIdentities()
	providers := authManager.GetProviders()

	if len(identities) == 0 {
		u.PrintfMarkdownToTUI("**No identities configured** in atmos.yaml\n")
		return nil
	}

	// Build options list.
	options := buildLogoutOptions(identities, providers)

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
	return executeLogoutOption(ctx, authManager, selected, dryRun, deleteKeychain, force)
}

func performLogoutAll(ctx context.Context, authManager auth.AuthManager, dryRun bool, deleteKeychain bool, force bool) error {
	if dryRun {
		u.PrintfMarkdownToTUI("**Dry run mode:** No credentials will be removed\n\n")
		u.PrintfMarkdownToTUI("**Would remove:**\n")
		if deleteKeychain {
			u.PrintfMessageToTUI("  • All identity keyring entries\n")
			u.PrintfMessageToTUI("  • All provider keyring entries\n")
		}

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

	// If deleteKeychain is requested, confirm with user.
	if deleteKeychain {
		isTTY := term.IsTTYSupportForStdout()
		confirmed, err := confirmKeychainDeletion("all identities", force, isTTY)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil // User cancelled, exit cleanly.
		}
	}

	// Perform logout.
	if err := authManager.LogoutAll(ctx, deleteKeychain); err != nil {
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

	// Display external credential warnings.
	displayExternalCredentialWarnings()

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
	authLogoutCmd.Flags().Bool("all", false, "Logout from all identities and providers")
	authLogoutCmd.Flags().Bool("dry-run", false, "Preview what would be removed without deleting")
	authLogoutCmd.Flags().Bool("keychain", false, "Also remove credentials from system keychain (destructive, requires confirmation)")
	authLogoutCmd.Flags().Bool("force", false, "Skip confirmation prompts (useful for CI/CD)")
	authCmd.AddCommand(authLogoutCmd)
}
