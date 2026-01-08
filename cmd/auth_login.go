package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui/theme"
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
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitConfig, err)
	}
	defer perf.Track(&atmosConfig, "cmd.executeAuthLoginCommand")()

	// Create auth manager.
	authManager, err := createAuthManager(&atmosConfig.Auth)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Check if --provider flag was provided.
	providerName, _ := cmd.Flags().GetString("provider")

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
		// Try identity-level authentication first.
		var needsProviderFallback bool
		whoami, needsProviderFallback, err = authenticateIdentity(ctx, cmd, authManager)

		if needsProviderFallback {
			// No identities available - fall back to provider authentication.
			// This enables seamless first-login with auto_provision_identities.
			providerName, err = getProviderForFallback(authManager)
			if err != nil {
				return err
			}
			whoami, err = authManager.AuthenticateProvider(ctx, providerName)
			if err != nil {
				return fmt.Errorf("%w: provider=%s: %w", errUtils.ErrAuthenticationFailed, providerName, err)
			}
		} else if err != nil {
			return err
		}
	}

	// Display success message using Atmos theme.
	displayAuthSuccess(whoami)

	return nil
}

// authenticateIdentity handles identity-level authentication with default and interactive selection.
// Returns (WhoamiInfo, needsProviderFallback, error) where needsProviderFallback indicates whether
// to fall back to provider-level authentication (when no identities are available).
func authenticateIdentity(ctx context.Context, cmd *cobra.Command, authManager auth.AuthManager) (*authTypes.WhoamiInfo, bool, error) {
	// Get identity from flag or use default.
	// Use centralized function that handles Cobra's NoOptDefVal quirk correctly.
	identityName := GetIdentityFromFlags(cmd, os.Args)

	// Check if user wants to interactively select identity.
	forceSelect := identityName == IdentityFlagSelectValue

	// If no identity specified, get the default identity (which prompts if needed).
	// If --identity flag was used without value, forceSelect will be true.
	if identityName == "" || forceSelect {
		var err error
		identityName, err = authManager.GetDefaultIdentity(forceSelect)
		if err != nil {
			// Check if we should fall back to provider-based auth.
			// This happens when no identities are available (e.g., first login with auto_provision_identities).
			if errors.Is(err, errUtils.ErrNoIdentitiesAvailable) ||
				errors.Is(err, errUtils.ErrNoDefaultIdentity) {
				return nil, true, nil
			}
			return nil, false, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrDefaultIdentity, err)
		}
	}

	// Perform identity authentication.
	whoami, err := authManager.Authenticate(ctx, identityName)
	if err != nil {
		return nil, false, fmt.Errorf("%w: identity=%s: %w", errUtils.ErrAuthenticationFailed, identityName, err)
	}

	return whoami, false, nil
}

// CreateAuthManager creates a new auth manager with all required dependencies.
// Exported for use by command packages (e.g., terraform package).
func CreateAuthManager(authConfig *schema.AuthConfig) (auth.AuthManager, error) {
	return createAuthManager(authConfig)
}

// createAuthManager creates a new auth manager with all required dependencies.
func createAuthManager(authConfig *schema.AuthConfig) (auth.AuthManager, error) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	return auth.NewAuthManager(authConfig, credStore, validator, nil)
}

const (
	secondsPerMinute = 60
	minutesPerHour   = 60
)

// formatDuration formats a duration into a human-readable string.
func formatDuration(d time.Duration) string {
	if d < 0 {
		return "expired"
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % minutesPerHour
	seconds := int(d.Seconds()) % secondsPerMinute

	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// displayAuthSuccess displays a styled success message with authentication details.
func displayAuthSuccess(whoami *authTypes.WhoamiInfo) {
	// Display checkmark with success message.
	u.PrintfMessageToTUI("\n%s Authentication successful!\n\n", theme.Styles.Checkmark)

	// Build table rows.
	var rows [][]string
	rows = append(rows, []string{"Provider", whoami.Provider})
	rows = append(rows, []string{"Identity", whoami.Identity})

	if whoami.Account != "" {
		rows = append(rows, []string{"Account", whoami.Account})
	}

	if whoami.Region != "" {
		rows = append(rows, []string{"Region", whoami.Region})
	}

	if whoami.Expiration != nil {
		expiresStr := whoami.Expiration.Format("2006-01-02 15:04:05 MST")
		duration := formatDuration(time.Until(*whoami.Expiration))
		// Style duration with darker gray.
		durationStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))
		expiresStr = fmt.Sprintf("%s %s", expiresStr, durationStyle.Render(fmt.Sprintf("(%s)", duration)))
		rows = append(rows, []string{"Expires", expiresStr})
	}

	// Create minimal charmbracelet table.
	// Note: Padding variation across platforms was causing snapshot test failures.
	// The table auto-sizes columns but the final width varied (Linux: 40 chars, macOS: 45 chars).
	// Removed `.Width()` constraint as it was causing word-wrapping issues.
	t := table.New().
		Rows(rows...).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderRow(false).
		BorderColumn(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			if col == 0 {
				// Key column - use cyan color.
				return lipgloss.NewStyle().
					Foreground(lipgloss.Color(theme.ColorCyan)).
					Padding(0, 1, 0, 2)
			}
			// Value column - default color with padding.
			return lipgloss.NewStyle().Padding(0, 1)
		})

	fmt.Fprintf(os.Stderr, "%s\n\n", t)
}

func init() {
	authLoginCmd.Flags().StringP("provider", "p", "", "Provider name to authenticate with (for SSO auto-provisioning)")
	authCmd.AddCommand(authLoginCmd)
}

// providerLister is an interface for listing providers (subset of auth.AuthManager).
type providerLister interface {
	ListProviders() []string
}

// isInteractive checks if we're running in an interactive terminal.
// Interactive mode requires stdin to be a TTY (for user input) and must not be in CI.
func isInteractive() bool {
	return term.IsTTYSupportForStdin() && !telemetry.IsCI()
}

// getProviderForFallback determines which provider to use when no identities are configured.
// If only one provider exists, it is auto-selected.
// If multiple providers exist and interactive, prompts user.
// If multiple providers exist and non-interactive, returns error with helpful message.
func getProviderForFallback(authManager providerLister) (string, error) {
	providers := authManager.ListProviders()

	if len(providers) == 0 {
		return "", errUtils.ErrNoProvidersAvailable
	}

	// Auto-select if only one provider.
	if len(providers) == 1 {
		return providers[0], nil
	}

	// Multiple providers - need interactive selection or error.
	if !isInteractive() {
		return "", fmt.Errorf("%w: use --provider flag to specify which provider", errUtils.ErrNoDefaultProvider)
	}

	return promptForProvider("No identities configured. Select a provider:", providers)
}

// promptForProvider prompts the user to select a provider from the given list.
func promptForProvider(message string, providers []string) (string, error) {
	if len(providers) == 0 {
		return "", errUtils.ErrNoProvidersAvailable
	}

	// Sort providers alphabetically for consistent ordering.
	sortedProviders := make([]string, len(providers))
	copy(sortedProviders, providers)
	sort.Strings(sortedProviders)

	var selectedProvider string

	// Create custom keymap that adds ESC to quit keys.
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(message).
				Description("Press ctrl+c or esc to exit").
				Options(huh.NewOptions(sortedProviders...)...).
				Value(&selectedProvider),
		),
	).WithKeyMap(keyMap)

	if err := form.Run(); err != nil {
		// Check if user aborted (Ctrl+C, ESC, etc.).
		if errors.Is(err, huh.ErrUserAborted) {
			return "", errUtils.ErrUserAborted
		}
		return "", fmt.Errorf("%w: %w", errUtils.ErrUnsupportedInputType, err)
	}

	return selectedProvider, nil
}
