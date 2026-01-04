package auth

import (
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	secondsPerMinute = 60
	minutesPerHour   = 60
)

// BuildConfigAndStacksInfo parses global flags and builds ConfigAndStacksInfo.
// This ensures auth commands honor global flags like --base-path, --config, --config-path, and --profile.
// Wraps flags.BuildConfigAndStacksInfo for convenience.
func BuildConfigAndStacksInfo(cmd *cobra.Command, v *viper.Viper) schema.ConfigAndStacksInfo {
	defer perf.Track(nil, "auth.BuildConfigAndStacksInfo")()

	return flags.BuildConfigAndStacksInfo(cmd, v)
}

// CreateAuthManager creates a new auth manager with all required dependencies.
// Exported for use by command packages (e.g., terraform package).
func CreateAuthManager(authConfig *schema.AuthConfig) (auth.AuthManager, error) {
	defer perf.Track(nil, "auth.CreateAuthManager")()

	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	return auth.NewAuthManager(authConfig, credStore, validator, nil)
}

// formatDuration formats a duration into a human-readable string.
func formatDuration(d time.Duration) string {
	defer perf.Track(nil, "auth.formatDuration")()

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
	defer perf.Track(nil, "auth.displayAuthSuccess")()

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

// handleHelpRequest handles help request for auth commands.
// Note: Calls os.Exit() per Cobra convention for interactive CLI help requests.
//
//nolint:revive // deep-exit is intentional for help handling
func handleHelpRequest(cmd *cobra.Command, args []string) {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			_ = cmd.Help()
			os.Exit(0)
		}
	}
}
