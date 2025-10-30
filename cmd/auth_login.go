package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
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
		return errors.Join(errUtils.ErrFailedToInitConfig, err)
	}
	defer perf.Track(&atmosConfig, "cmd.executeAuthLoginCommand")()

	// Create auth manager.
	authManager, err := createAuthManager(&atmosConfig.Auth)
	if err != nil {
		return errors.Join(errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Get identity from flag or use default.
	// IMPORTANT: When BindPFlag is used, Viper can override flag values. To ensure
	// command-line flags take precedence, we check if the flag was explicitly set.
	// If Changed() returns true, the user provided the flag on the command line.
	var identityName string
	if cmd.Flags().Changed(IdentityFlagName) {
		// Flag was explicitly provided on command line (either with or without value).
		// GetString() will return the command-line value or NoOptDefVal (__SELECT__).
		identityName, _ = cmd.Flags().GetString(IdentityFlagName)
	} else {
		// Flag not provided on command line - fall back to viper (config/env/defaults).
		identityName = viper.GetString(IdentityFlagName)
	}

	// Check if user wants to interactively select identity.
	forceSelect := identityName == IdentityFlagSelectValue

	// If no identity specified, get the default identity (which prompts if needed).
	// If --identity flag was used without value, forceSelect will be true.
	if identityName == "" || forceSelect {
		identityName, err = authManager.GetDefaultIdentity(forceSelect)
		if err != nil {
			return errors.Join(errUtils.ErrDefaultIdentity, err)
		}
	}

	// Perform authentication.
	ctx := context.Background()
	whoami, err := authManager.Authenticate(ctx, identityName)
	if err != nil {
		return errors.Join(errUtils.ErrAuthenticationFailed, fmt.Errorf("identity=%s: %w", identityName, err))
	}

	// Display success message using Atmos theme.
	displayAuthSuccess(whoami)

	return nil
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
	checkMark := theme.Styles.Checkmark
	fmt.Fprintf(os.Stderr, "\n%s Authentication successful!\n\n", checkMark)

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
	authCmd.AddCommand(authLoginCmd)
}
