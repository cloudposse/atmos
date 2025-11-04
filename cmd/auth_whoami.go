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
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	logKeyIdentity = "identity"
)

var authWhoamiParser = flags.NewStandardOptionsBuilder().
	WithOutput("", "table", "json").
	Build()

// authWhoamiCmd shows current authentication status.
var authWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current authentication status",
	Long:  "Display information about the current effective authentication principal.",
	RunE:  executeAuthWhoamiCommand,
}

func executeAuthWhoamiCommand(cmd *cobra.Command, args []string) error {
	handleHelpRequest(cmd, args)

	// Parse flags using StandardOptions.
	opts, err := authWhoamiParser.Parse(context.Background(), args)
	if err != nil {
		return err
	}

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
	ctx := context.Background()
	whoami, err := authManager.Whoami(ctx, identityName)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	// Validate credentials if available.
	isValid := validateCredentials(ctx, whoami)

	// Output.
	if opts.Output == "json" {
		return printWhoamiJSON(whoami)
	}
	printWhoamiHuman(whoami, isValid)
	return nil
}

// validateCredentials attempts to validate the credentials and returns true if valid.
// If validation succeeds, it populates whoami with additional info (principal, expiration, etc.).
func validateCredentials(ctx context.Context, whoami *authTypes.WhoamiInfo) bool {
	if whoami.Credentials == nil {
		log.Debug("Validation failed: no credentials in WhoamiInfo", "identity", whoami.Identity)
		return false
	}

	// Try to validate using the Validate method if available.
	type validator interface {
		Validate(context.Context) (*authTypes.ValidationInfo, error)
	}

	v, ok := whoami.Credentials.(validator)
	if !ok {
		// If no validator, check expiration as fallback.
		expired := whoami.Credentials.IsExpired()
		log.Debug("Credential validation using expiration check", logKeyIdentity, whoami.Identity, "expired", expired)
		return !expired
	}

	validationInfo, err := v.Validate(ctx)
	if err != nil {
		log.Debug("Credential validation failed", logKeyIdentity, whoami.Identity, "error", err)
		return false
	}

	log.Debug("Credential validation succeeded", logKeyIdentity, whoami.Identity)

	// Populate whoami with validation info.
	populateWhoamiFromValidation(whoami, validationInfo)

	return true
}

// populateWhoamiFromValidation populates whoami info from validation results.
func populateWhoamiFromValidation(whoami *authTypes.WhoamiInfo, validationInfo *authTypes.ValidationInfo) {
	if validationInfo == nil {
		return
	}
	if validationInfo.Principal != "" {
		whoami.Principal = validationInfo.Principal
	}
	if validationInfo.Account != "" {
		whoami.Account = validationInfo.Account
	}
	if validationInfo.Expiration != nil {
		whoami.Expiration = validationInfo.Expiration
	}
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
	// Get identity from flag or use default.
	// Use GetIdentityFromFlags which handles Cobra's NoOptDefVal quirk correctly.
	identityName := GetIdentityFromFlags(cmd, os.Args)

	// Check if user wants to interactively select identity.
	forceSelect := identityName == IdentityFlagSelectValue

	if identityName != "" && !forceSelect {
		return identityName, nil
	}

	defaultIdentity, err := authManager.GetDefaultIdentity(forceSelect)
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

func printWhoamiHuman(whoami *authTypes.WhoamiInfo, isValid bool) {
	// Display status indicator with colored checkmark or X.
	statusIndicator := theme.Styles.XMark.String()
	if isValid {
		statusIndicator = theme.Styles.Checkmark.String()
	}

	fmt.Fprintf(os.Stderr, "%s Current Authentication Status\n\n", statusIndicator)

	// Build and print table.
	rows := buildWhoamiTableRows(whoami)
	t := createWhoamiTable(rows)

	fmt.Fprintf(os.Stderr, "%s\n", t)
}

// buildWhoamiTableRows builds table rows for whoami output.
func buildWhoamiTableRows(whoami *authTypes.WhoamiInfo) [][]string {
	const expiringThresholdMinutes = 15

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

	if whoami.Expiration != nil {
		expiresStr := formatExpiration(whoami.Expiration, expiringThresholdMinutes)
		rows = append(rows, []string{"Expires", expiresStr})
	}

	rows = append(rows, []string{"Last Updated", whoami.LastUpdated.Format("2006-01-02 15:04:05 MST")})

	return rows
}

// formatExpiration formats expiration time with duration and styling.
func formatExpiration(expiration *time.Time, thresholdMinutes int) string {
	expiresStr := expiration.Format("2006-01-02 15:04:05 MST")
	duration := formatDuration(time.Until(*expiration))

	timeUntilExpiration := time.Until(*expiration)
	var durationStyle lipgloss.Style
	if timeUntilExpiration > 0 && timeUntilExpiration < time.Duration(thresholdMinutes)*time.Minute {
		durationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorRed))
	} else {
		durationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))
	}

	return fmt.Sprintf("%s %s", expiresStr, durationStyle.Render(fmt.Sprintf("(%s)", duration)))
}

// createWhoamiTable creates a formatted table for whoami output.
func createWhoamiTable(rows [][]string) *table.Table {
	return table.New().
		Rows(rows...).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderRow(false).
		BorderColumn(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			if col == 0 {
				return lipgloss.NewStyle().
					Foreground(lipgloss.Color(theme.ColorCyan)).
					Padding(0, 1, 0, 2)
			}
			return lipgloss.NewStyle().Padding(0, 1)
		})
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
	// Register StandardOptions flags.
	authWhoamiParser.RegisterFlags(authWhoamiCmd)
	_ = authWhoamiParser.BindToViper(viper.GetViper())

	// Register flag completion for output.
	if err := authWhoamiCmd.RegisterFlagCompletionFunc("output", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		log.Trace("Failed to register output flag completion", "error", err)
	}

	authCmd.AddCommand(authWhoamiCmd)
}
