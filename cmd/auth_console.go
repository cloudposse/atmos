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
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
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
	handleHelpRequest(cmd, args)

	// Load atmos config.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf("%w: failed to load atmos config: %w", errUtils.ErrAuthConsole, err)
	}

	// Create auth manager.
	authManager, err := createAuthManager(&atmosConfig.Auth)
	if err != nil {
		return fmt.Errorf("%w: failed to create auth manager: %w", errUtils.ErrAuthConsole, err)
	}

	// Get identity from flag or use default.
	identityName, _ := cmd.Flags().GetString(IdentityFlagName)
	if identityName == "" {
		identityName, err = authManager.GetDefaultIdentity()
		if err != nil {
			return fmt.Errorf("%w: failed to get default identity: %w", errUtils.ErrAuthConsole, err)
		}
		if identityName == "" {
			return fmt.Errorf("%w: no default identity configured", errUtils.ErrAuthConsole)
		}
	}

	// Authenticate to get credentials.
	ctx := context.Background()
	whoami, err := authManager.Authenticate(ctx, identityName)
	if err != nil {
		return fmt.Errorf("%w: authentication failed: %w", errUtils.ErrAuthConsole, err)
	}

	// Retrieve credentials: use in-memory first, then retrieve from store.
	var creds types.ICredentials
	if whoami.Credentials != nil {
		creds = whoami.Credentials
	} else if whoami.CredentialsRef != "" {
		credStore := credentials.NewCredentialStore()
		creds, err = credStore.Retrieve(whoami.CredentialsRef)
		if err != nil {
			return fmt.Errorf("%w: failed to retrieve credentials from store: %w", errUtils.ErrAuthConsole, err)
		}
	} else {
		return fmt.Errorf("%w: no credentials available", errUtils.ErrAuthConsole)
	}

	// Check if provider supports console access and get the console URL generator.
	consoleProvider, err := getConsoleProvider(authManager, whoami.Identity)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrAuthConsole, err)
	}

	// Generate console URL.
	options := types.ConsoleURLOptions{
		Destination:     consoleDestination,
		SessionDuration: consoleDuration,
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

	// Print formatted output using lipgloss (without URL unless there's an error).
	printConsoleInfo(whoami, duration, false, "")

	// Open in browser unless skipped.
	if !consoleSkipOpen {
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
		// User explicitly skipped opening, so show the URL.
		fmt.Fprintf(os.Stderr, "\n")
		printConsoleURL(consoleURL)
	}

	return nil
}

// printConsoleInfo prints formatted console information using lipgloss.
func printConsoleInfo(whoami *types.WhoamiInfo, duration time.Duration, showURL bool, consoleURL string) {
	// Define styles.
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan)).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray)).Width(18)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorWhite))

	// Print header.
	fmt.Fprintf(os.Stderr, "\n%s\n\n", headerStyle.Render("Console URL Generated"))

	// Print fields.
	fmt.Fprintf(os.Stderr, "%s %s\n", labelStyle.Render("Provider:"), valueStyle.Render(whoami.Provider))
	fmt.Fprintf(os.Stderr, "%s %s\n", labelStyle.Render("Identity:"), valueStyle.Render(whoami.Identity))

	if whoami.Account != "" {
		fmt.Fprintf(os.Stderr, "%s %s\n", labelStyle.Render("Account:"), valueStyle.Render(whoami.Account))
	}

	if duration > 0 {
		// Calculate expiration time.
		expiresAt := time.Now().Add(duration)
		fmt.Fprintf(os.Stderr, "%s %s\n", labelStyle.Render("Session Duration:"), valueStyle.Render(duration.String()))
		fmt.Fprintf(os.Stderr, "%s %s\n", labelStyle.Render("Session Expires:"), valueStyle.Render(expiresAt.Format("2006-01-02 15:04:05 MST")))
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
		return nil, fmt.Errorf("Azure console access not yet implemented (coming soon)")
	case types.ProviderKindGCPOIDC:
		return nil, fmt.Errorf("GCP console access not yet implemented (coming soon)")
	default:
		return nil, fmt.Errorf("provider %q does not support web console access", providerKind)
	}
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
