package cmd

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	awsAuth "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	// ConsoleLabelWidth is the width for label styling in console output.
	consoleLabelWidth = 18
	// ConsoleOutputFormat is the format string for label-value pairs.
	consoleOutputFormat = "%s %s\n"
)

var (
	consoleDestination string
	consoleDuration    time.Duration
	consoleIssuer      string
	consolePrintOnly   bool
	consoleSkipOpen    bool
)

//go:embed markdown/atmos_auth_console_usage.md
var authConsoleUsageMarkdown string

// authConsoleCmd opens the cloud provider web console using authenticated credentials.
var authConsoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Open cloud provider web console in browser",
	Long: `Open the cloud provider web console in your default browser using authenticated credentials.

This command generates a temporary console sign-in URL using your authenticated identity's
credentials and opens it in your default browser. Supports AWS, Azure, GCP, and other providers
that implement console access.`,
	Example:            authConsoleUsageMarkdown,
	RunE:               executeAuthConsoleCommand,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func executeAuthConsoleCommand(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "cmd.executeAuthConsoleCommand")()

	handleHelpRequest(cmd, args)

	// Initialize auth manager.
	authManager, err := initializeAuthManager()
	if err != nil {
		return err
	}

	// Get identity name.
	identityName, err := resolveIdentityName(cmd, authManager)
	if err != nil {
		return err
	}

	// Try to use cached credentials first (passive check, no prompts).
	// Only authenticate if cached credentials are not available or expired.
	ctx := context.Background()
	whoami, err := authManager.GetCachedCredentials(ctx, identityName)
	if err != nil {
		log.Debug("No valid cached credentials found, authenticating", "identity", identityName, "error", err)
		// No valid cached credentials - perform full authentication.
		whoami, err = authManager.Authenticate(ctx, identityName)
		if err != nil {
			return fmt.Errorf("%w: authentication failed: %w", errUtils.ErrAuthConsole, err)
		}
	}

	// Retrieve credentials.
	creds, err := retrieveCredentials(whoami)
	if err != nil {
		return err
	}

	// Check if provider supports console access and get the console URL generator.
	consoleProvider, err := getConsoleProvider(authManager, whoami.Identity)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrAuthConsole, err)
	}

	// Resolve session duration (flag takes precedence over provider config).
	sessionDuration, err := resolveConsoleDuration(cmd, authManager, whoami.Provider)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrAuthConsole, err)
	}

	// Generate console URL.
	options := types.ConsoleURLOptions{
		Destination:     consoleDestination,
		SessionDuration: sessionDuration,
		Issuer:          consoleIssuer,
		OpenInBrowser:   !consoleSkipOpen && !consolePrintOnly,
	}

	consoleURL, duration, err := consoleProvider.GetConsoleURL(ctx, creds, options)
	if err != nil {
		return fmt.Errorf("%w: failed to generate console URL: %w", errUtils.ErrAuthConsole, err)
	}

	if consolePrintOnly {
		// Print to stdout for piping.
		fmt.Println(consoleURL)
		return nil
	}

	// Print formatted output and handle browser opening.
	printConsoleInfo(whoami, duration, false, "")
	handleBrowserOpen(consoleURL)

	return nil
}

// handleBrowserOpen handles opening the console URL in the browser or displaying it.
func handleBrowserOpen(consoleURL string) {
	if !consoleSkipOpen && !telemetry.IsCI() {
		fmt.Fprintf(os.Stderr, "\n")
		if err := u.OpenUrl(consoleURL); err != nil {
			// Show URL on error so user can manually open it.
			printConsoleURL(consoleURL)
			warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorOrange)).Bold(true)
			fmt.Fprintf(os.Stderr, "\n%s Failed to open browser: %v\n", warningStyle.Render("Warning:"), err)
			fmt.Fprintf(os.Stderr, "Please copy the URL above and open it manually.\n")
		} else {
			successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
			fmt.Fprintf(os.Stderr, "%s\n", successStyle.Render("✓ Opened console in browser"))
		}
	} else {
		// User explicitly skipped opening or running in CI, so show the URL.
		fmt.Fprintf(os.Stderr, "\n")
		printConsoleURL(consoleURL)
	}
}

// printConsoleInfo prints formatted console information using lipgloss.
func printConsoleInfo(whoami *types.WhoamiInfo, duration time.Duration, showURL bool, consoleURL string) {
	// Define styles.
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan)).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray)).Width(consoleLabelWidth)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorWhite))

	// Print header.
	fmt.Fprintf(os.Stderr, "\n%s\n\n", headerStyle.Render("Console URL Generated"))

	// Print fields.
	fmt.Fprintf(os.Stderr, consoleOutputFormat, labelStyle.Render("Provider:"), valueStyle.Render(whoami.Provider))
	fmt.Fprintf(os.Stderr, consoleOutputFormat, labelStyle.Render("Identity:"), valueStyle.Render(whoami.Identity))

	if whoami.Account != "" {
		fmt.Fprintf(os.Stderr, consoleOutputFormat, labelStyle.Render("Account:"), valueStyle.Render(whoami.Account))
	}

	if duration > 0 {
		// Calculate expiration time.
		expiresAt := time.Now().Add(duration)
		fmt.Fprintf(os.Stderr, consoleOutputFormat, labelStyle.Render("Session Duration:"), valueStyle.Render(duration.String()))
		fmt.Fprintf(os.Stderr, consoleOutputFormat, labelStyle.Render("Session Expires:"), valueStyle.Render(expiresAt.Format("2006-01-02 15:04:05 MST")))
	}

	// Only print URL if requested (for error cases or --no-open).
	if showURL && consoleURL != "" {
		printConsoleURL(consoleURL)
	}
}

// printConsoleURL prints the console URL in a formatted way.
func printConsoleURL(consoleURL string) {
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray))
	urlStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan))
	fmt.Fprintf(os.Stderr, "\n%s\n%s\n", labelStyle.Render("Console URL:"), urlStyle.Render(consoleURL))
}

// getConsoleProvider returns a ConsoleAccessProvider for the given identity.
func getConsoleProvider(authManager types.AuthManager, identityName string) (types.ConsoleAccessProvider, error) {
	defer perf.Track(nil, "cmd.getConsoleProvider")()

	// Get provider kind for the identity.
	providerKind, err := authManager.GetProviderKindForIdentity(identityName)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider kind: %w", err)
	}

	// Check if provider supports console access based on kind.
	switch providerKind {
	case types.ProviderKindAWSIAMIdentityCenter, types.ProviderKindAWSSAML:
		// Return AWS console URL generator with default HTTP client.
		generator := awsAuth.NewConsoleURLGenerator(nil)
		return generator, nil
	case types.ProviderKindAzureOIDC:
		return nil, fmt.Errorf("%w: Azure console access not yet implemented (coming soon)", errUtils.ErrProviderNotSupported)
	case types.ProviderKindGCPOIDC:
		return nil, fmt.Errorf("%w: GCP console access not yet implemented (coming soon)", errUtils.ErrProviderNotSupported)
	default:
		return nil, fmt.Errorf("%w: provider %q does not support web console access", errUtils.ErrProviderNotSupported, providerKind)
	}
}

// initializeAuthManager loads config and creates the auth manager.
func initializeAuthManager() (types.AuthManager, error) {
	defer perf.Track(nil, "cmd.initializeAuthManager")()

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load atmos config: %w", errUtils.ErrAuthConsole, err)
	}

	authManager, err := createAuthManager(&atmosConfig.Auth)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create auth manager: %w", errUtils.ErrAuthConsole, err)
	}

	return authManager, nil
}

// resolveIdentityName gets identity from flag or uses default.
func resolveIdentityName(cmd *cobra.Command, authManager types.AuthManager) (string, error) {
	defer perf.Track(nil, "cmd.resolveIdentityName")()

	identityName, _ := cmd.Flags().GetString(IdentityFlagName)
	if identityName != "" {
		return identityName, nil
	}

	identityName, err := authManager.GetDefaultIdentity()
	if err != nil {
		return "", fmt.Errorf("%w: failed to get default identity: %w", errUtils.ErrAuthConsole, err)
	}

	if identityName == "" {
		return "", fmt.Errorf("%w: no default identity configured", errUtils.ErrAuthConsole)
	}

	return identityName, nil
}

// retrieveCredentials retrieves credentials from whoami info.
func retrieveCredentials(whoami *types.WhoamiInfo) (types.ICredentials, error) {
	defer perf.Track(nil, "cmd.retrieveCredentials")()

	switch {
	case whoami.Credentials != nil:
		return whoami.Credentials, nil
	case whoami.CredentialsRef != "":
		credStore := credentials.NewCredentialStore()
		creds, err := credStore.Retrieve(whoami.CredentialsRef)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to retrieve credentials from store: %w", errUtils.ErrAuthConsole, err)
		}
		return creds, nil
	default:
		return nil, fmt.Errorf("%w: no credentials available", errUtils.ErrAuthConsole)
	}
}

// resolveConsoleDuration resolves console session duration from flag or provider config.
// Flag takes precedence over provider configuration.
func resolveConsoleDuration(cmd *cobra.Command, authManager types.AuthManager, providerName string) (time.Duration, error) {
	defer perf.Track(nil, "cmd.resolveConsoleDuration")()

	// Check if flag was explicitly set by user.
	if cmd.Flags().Changed("duration") {
		return consoleDuration, nil
	}

	// Get provider configuration.
	providers := authManager.GetProviders()
	provider, exists := providers[providerName]
	if !exists {
		// No provider config found, use default from flag.
		return consoleDuration, nil
	}

	// Check if provider has console configuration.
	if provider.Console == nil || provider.Console.SessionDuration == "" {
		// No console config, use default from flag.
		return consoleDuration, nil
	}

	// Parse provider's session duration.
	duration, err := time.ParseDuration(provider.Console.SessionDuration)
	if err != nil {
		return 0, fmt.Errorf("invalid session_duration in provider %q console config: %w", providerName, err)
	}

	return duration, nil
}

func init() {
	authConsoleCmd.Flags().StringVar(&consoleDestination, "destination", "",
		"Console page to navigate to. Supports full URLs or shorthand aliases like 's3', 'ec2', 'lambda', etc.")
	authConsoleCmd.Flags().DurationVar(&consoleDuration, "duration", 1*time.Hour,
		"Console session duration (provider may have max limits)")
	authConsoleCmd.Flags().StringVar(&consoleIssuer, "issuer", "atmos",
		"Issuer identifier for the console session (AWS only)")
	authConsoleCmd.Flags().BoolVar(&consolePrintOnly, "print-only", false,
		"Print the console URL to stdout without opening browser")
	authConsoleCmd.Flags().BoolVar(&consoleSkipOpen, "no-open", false,
		"Generate URL but don't open browser automatically")

	// Register autocomplete for destination flag (AWS service aliases).
	if err := authConsoleCmd.RegisterFlagCompletionFunc("destination", destinationFlagCompletion); err != nil {
		log.Trace("Failed to register destination flag completion", "error", err)
	}

	// Register autocomplete for identity flag.
	AddIdentityCompletion(authConsoleCmd)

	authCmd.AddCommand(authConsoleCmd)
}

// destinationFlagCompletion provides shell completion for the --destination flag.
// Returns AWS service aliases for autocomplete suggestions.
func destinationFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Get all available AWS service aliases.
	aliases := awsAuth.GetAvailableAliases()
	return aliases, cobra.ShellCompDirectiveNoFileComp
}
