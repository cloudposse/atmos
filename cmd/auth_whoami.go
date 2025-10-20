package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// authWhoamiCmd shows current authentication status.
var authWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current authentication status",
	Long:  "Display information about the current effective authentication principal.",

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE:               executeAuthWhoamiCommand,
}

func executeAuthWhoamiCommand(cmd *cobra.Command, args []string) error {
	handleHelpRequest(cmd, args)

	// Load atmos config and auth manager.
	authManager, err := loadAuthManager()
	if err != nil {
		return err
	}

	// Determine identity.
	identityName, err := identityFromFlagOrDefault(cmd, authManager)
	if err != nil {
		return err
	}

	// Query whoami.
	whoami, err := authManager.Whoami(context.Background(), identityName)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	// Output.
	if viper.GetString("auth.whoami.output") == "json" {
		return printWhoamiJSON(whoami)
	}
	printWhoamiHuman(whoami)
	return nil
}

func loadAuthManager() (authTypes.AuthManager, error) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load atmos config: %v", errUtils.ErrInvalidAuthConfig, err)
	}
	manager, err := createAuthManager(&atmosConfig.Auth)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errUtils.ErrInvalidAuthConfig, err)
	}
	return manager, nil
}

func identityFromFlagOrDefault(cmd *cobra.Command, authManager authTypes.AuthManager) (string, error) {
	identityName, _ := cmd.Flags().GetString("identity")
	if identityName != "" {
		return identityName, nil
	}
	defaultIdentity, err := authManager.GetDefaultIdentity()
	if err != nil {
		return "", fmt.Errorf("%w: no default identity configured and no identity specified: %v", errUtils.ErrInvalidAuthConfig, err)
	}
	return defaultIdentity, nil
}

func printWhoamiJSON(whoami *authTypes.WhoamiInfo) error {
	// Redact home directory in environment variable values before output.
	redactedWhoami := *whoami
	// Never emit credentials in JSON output.
	redactedWhoami.Credentials = nil
	homeDir, _ := homedir.Dir()
	if whoami.Environment != nil {
		redactedWhoami.Environment = sanitizeEnvMap(whoami.Environment, homeDir)
	}
	jsonData, err := json.MarshalIndent(redactedWhoami, "", "  ")
	if err != nil {
		errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, "Failed to marshal JSON", "")
		return errUtils.ErrInvalidAuthConfig
	}
	fmt.Println(string(jsonData))
	return nil
}

func printWhoamiHuman(whoami *authTypes.WhoamiInfo) {
	const (
		expiringThresholdMinutes = 15
	)

	fmt.Fprintf(os.Stderr, "Current Authentication Status\n\n")

	// Build table rows.
	var rows [][]string
	rows = append(rows, []string{"Provider", whoami.Provider})
	rows = append(rows, []string{"Identity", whoami.Identity})

	if whoami.Principal != "" {
		rows = append(rows, []string{"Principal", whoami.Principal})
	}

	if whoami.Account != "" {
		rows = append(rows, []string{"Account", whoami.Account})
	}

	if whoami.Region != "" {
		rows = append(rows, []string{"Region", whoami.Region})
	}

	// Track if credentials are expiring soon.
	var expiringSoon bool
	if whoami.Expiration != nil {
		expiresStr := whoami.Expiration.Format("2006-01-02 15:04:05 MST")
		duration := formatDuration(time.Until(*whoami.Expiration))

		// Check if expiring within threshold.
		timeUntilExpiration := time.Until(*whoami.Expiration)
		if timeUntilExpiration > 0 && timeUntilExpiration < expiringThresholdMinutes*time.Minute {
			expiringSoon = true
			expiresStr = fmt.Sprintf("%s (%s)", expiresStr, duration)
		} else {
			expiresStr = fmt.Sprintf("%s (%s)", expiresStr, duration)
		}

		rows = append(rows, []string{"Expires", expiresStr})
	}

	rows = append(rows, []string{"Last Updated", whoami.LastUpdated.Format("2006-01-02 15:04:05 MST")})

	// Create minimal charmbracelet table.
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

			// Value column - check if this is the Expires row and if credentials are expiring.
			if expiringSoon && row == len(rows)-2 && whoami.Expiration != nil {
				// Expires row with expiring credentials - use red for duration.
				return lipgloss.NewStyle().
					Foreground(lipgloss.Color(theme.ColorRed)).
					Padding(0, 1)
			}

			// Default color with padding.
			return lipgloss.NewStyle().Padding(0, 1)
		})

	fmt.Fprintf(os.Stderr, "%s\n\n", t)
}

// redactHomeDir replaces occurrences of the homeDir at the start of v with "~" to avoid leaking user paths.
func redactHomeDir(v string, homeDir string) string {
	if homeDir == "" {
		return v
	}
	// Ensure both have the same path separator.
	if strings.HasPrefix(v, homeDir+string(os.PathSeparator)) {
		return "~" + v[len(homeDir):]
	}
	if v == homeDir {
		return "~"
	}
	return v
}

// sanitizeEnvMap redacts secrets and user paths from environment variables.
func sanitizeEnvMap(in map[string]string, homeDir string) map[string]string {
	out := make(map[string]string, len(in))
	sensitive := []string{"SECRET", "TOKEN", "PASSWORD", "PRIVATE", "ACCESS_KEY", "SESSION"}
	for k, v := range in {
		redacted := redactHomeDir(v, homeDir)
		upperK := strings.ToUpper(k)
		for _, s := range sensitive {
			if strings.Contains(upperK, s) {
				redacted = "***REDACTED***"
				break
			}
		}
		out[k] = redacted
	}
	return out
}

func init() {
	authWhoamiCmd.Flags().StringP("output", "o", "", "Output format (json)")
	if err := viper.BindPFlag("auth.whoami.output", authWhoamiCmd.Flags().Lookup("output")); err != nil {
		log.Trace("Failed to bind auth.whoami.output flag", "error", err)
	}
	if err := viper.BindEnv("auth.whoami.output", "ATMOS_AUTH_WHOAMI_OUTPUT"); err != nil {
		log.Trace("Failed to bind auth.whoami.output environment variable", "error", err)
	}
	if err := authWhoamiCmd.RegisterFlagCompletionFunc("output", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		log.Trace("Failed to register output flag completion", "error", err)
	}
	authCmd.AddCommand(authWhoamiCmd)
}
