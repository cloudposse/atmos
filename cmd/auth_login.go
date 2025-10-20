package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
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

	// Load atmos config
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf("failed to load atmos config: %w", err)
	}

	// Create auth manager
	authManager, err := createAuthManager(&atmosConfig.Auth)
	if err != nil {
		return fmt.Errorf("failed to create auth manager: %w", err)
	}

	// Get identity from flag or use default
	identityName, _ := cmd.Flags().GetString("identity")

	// Perform authentication
	ctx := context.Background()
	whoami, err := authManager.Authenticate(ctx, identityName)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Display styled success message
	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorGreen)).
		Bold(true)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.ColorGreen)).
		Padding(1, 2).
		Width(60)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorDarkGray)).
		Width(12)

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorWhite)).
		Bold(true)

	checkmark := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorCheckmark)).
		SetString("✓")

	// Build success message content
	title := checkmark.Render() + " " + successStyle.Render("Authentication Successful!")

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")
	lines = append(lines, labelStyle.Render("Provider:")+" "+valueStyle.Render(whoami.Provider))
	lines = append(lines, labelStyle.Render("Identity:")+" "+valueStyle.Render(whoami.Identity))

	if whoami.Account != "" {
		lines = append(lines, labelStyle.Render("Account:")+" "+valueStyle.Render(whoami.Account))
	}
	if whoami.Region != "" {
		lines = append(lines, labelStyle.Render("Region:")+" "+valueStyle.Render(whoami.Region))
	}
	if whoami.Expiration != nil {
		expiresValue := whoami.Expiration.Format("2006-01-02 15:04:05 MST")
		lines = append(lines, labelStyle.Render("Expires:")+" "+valueStyle.Render(expiresValue))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	box := boxStyle.Render(content)

	fmt.Fprintln(os.Stderr, "\n"+box+"\n")

	return nil
}

// createAuthManager creates a new auth manager with all required dependencies.
func createAuthManager(authConfig *schema.AuthConfig) (auth.AuthManager, error) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	return auth.NewAuthManager(authConfig, credStore, validator, nil)
}

func init() {
	authCmd.AddCommand(authLoginCmd)
}
