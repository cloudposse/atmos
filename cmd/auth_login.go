package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
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
