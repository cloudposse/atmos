package auth

import (
	"context"
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
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	logKeyIdentity = "identity"
	// OutputFlagName is the name of the output flag for whoami command.
	OutputFlagName = "output"
)

// whoamiParser handles flags for the whoami command.
var whoamiParser *flags.StandardParser

// authWhoamiCmd shows current authentication status.
var authWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current authentication status",
	Long:  "Display information about the current effective authentication principal.",

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE:               executeAuthWhoamiCommand,
}

func init() {
	defer perf.Track(nil, "auth.whoami.init")()

	// Create parser with whoami-specific flags.
	whoamiParser = flags.NewStandardParser(
		flags.WithStringFlag(OutputFlagName, "o", "", "Output format (json)"),
		flags.WithEnvVars(OutputFlagName, "ATMOS_AUTH_WHOAMI_OUTPUT"),
		flags.WithValidValues(OutputFlagName, "", "json"),
	)

	// Register flags with the command.
	whoamiParser.RegisterFlags(authWhoamiCmd)

	// Bind to Viper for environment variable support.
	if err := whoamiParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register output flag completion.
	if err := authWhoamiCmd.RegisterFlagCompletionFunc(OutputFlagName, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		log.Trace("Failed to register output flag completion", "error", err)
	}

	// Add to parent command.
	authCmd.AddCommand(authWhoamiCmd)
}

func executeAuthWhoamiCommand(cmd *cobra.Command, args []string) error {
	handleHelpRequest(cmd, args)

	defer perf.Track(nil, "auth.executeAuthWhoamiCommand")()

	// Bind parsed flags to Viper for precedence.
	v := viper.GetViper()
	if err := whoamiParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Load atmos config and auth manager.
	authManager, err := loadAuthManager(cmd, v)
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
	if v.GetString(OutputFlagName) == "json" {
		return printWhoamiJSON(whoami)
	}
	printWhoamiHuman(whoami, isValid)
	return nil
}

// validateCredentials attempts to validate the credentials and returns true if valid.
// If validation succeeds, it populates whoami with additional info (principal, expiration, etc.).
func validateCredentials(ctx context.Context, whoami *authTypes.WhoamiInfo) bool {
	defer perf.Track(nil, "auth.validateCredentials")()

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
	defer perf.Track(nil, "auth.populateWhoamiFromValidation")()

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

func loadAuthManager(cmd *cobra.Command, v *viper.Viper) (authTypes.AuthManager, error) {
	defer perf.Track(nil, "auth.loadAuthManager")()

	// Parse global flags and build ConfigAndStacksInfo to honor --base-path, --config, --config-path, --profile.
	configAndStacksInfo := BuildConfigAndStacksInfo(cmd, v)

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load atmos config: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	manager, err := CreateAuthManager(&atmosConfig.Auth)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	return manager, nil
}

func identityFromFlagOrDefault(cmd *cobra.Command, authManager authTypes.AuthManager) (string, error) {
	defer perf.Track(nil, "auth.identityFromFlagOrDefault")()

	// Get identity from flag or use default.
	// Use GetIdentityFromFlags which handles Cobra's NoOptDefVal quirk correctly.
	identityName := GetIdentityFromFlags(cmd)

	// If flag wasn't provided, check Viper for env var fallback.
	if identityName == "" {
		identityName = viper.GetString(IdentityFlagName)
	}

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
	defer perf.Track(nil, "auth.printWhoamiJSON")()

	// Redact home directory in environment variable values before output.
	redactedWhoami := *whoami
	// Never emit credentials in JSON output.
	redactedWhoami.Credentials = nil
	homeDir, _ := homedir.Dir()
	if whoami.Environment != nil {
		redactedWhoami.Environment = sanitizeEnvMap(whoami.Environment, homeDir)
	}
	// Use data.WriteJSON() to write to the data channel (stdout) with proper I/O handling.
	// This ensures output goes through the I/O layer with automatic masking and respects
	// stream redirection in tests.
	return data.WriteJSON(redactedWhoami)
}

func printWhoamiHuman(whoami *authTypes.WhoamiInfo, isValid bool) {
	defer perf.Track(nil, "auth.printWhoamiHuman")()

	// Display status indicator with colored checkmark or X.
	statusIndicator := theme.Styles.XMark.String()
	if isValid {
		statusIndicator = theme.Styles.Checkmark.String()
	}

	_ = ui.Writef("%s Current Authentication Status\n\n", statusIndicator)

	// Build and print table.
	rows := buildWhoamiTableRows(whoami)
	t := createWhoamiTable(rows)

	_ = ui.Writef("%s\n", t)

	// Show warning with tip if credentials are invalid.
	if !isValid {
		_ = ui.Writeln("")
		_ = ui.Warning("Credentials may be expired or invalid.")
		_ = ui.Writef("  Run 'atmos auth login --identity %s' to refresh.\n", whoami.Identity)
	}
}

// buildWhoamiTableRows builds table rows for whoami output.
func buildWhoamiTableRows(whoami *authTypes.WhoamiInfo) [][]string {
	defer perf.Track(nil, "auth.buildWhoamiTableRows")()

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
	defer perf.Track(nil, "auth.formatExpiration")()

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
	defer perf.Track(nil, "auth.createWhoamiTable")()

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
	defer perf.Track(nil, "auth.sanitizeEnvMap")()

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
