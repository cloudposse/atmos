package theme

import (
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

//go:embed markdown/atmos_theme_list_usage.md
var themeListUsage string

// themeListParser is the flag parser for theme list command.
var themeListParser *flags.StandardFlagParser

// ThemeListOptions holds the options for theme list command.
type ThemeListOptions struct {
	RecommendedOnly bool
}

// themeListCmd lists available terminal themes.
var themeListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List available terminal themes",
	Long:    "Display available terminal themes that can be used for markdown rendering. By default shows all themes.",
	Example: themeListUsage,
	Args:    cobra.NoArgs,
	RunE:    executeThemeList,
}

func init() {
	// Create flag parser with recommended flag.
	themeListParser = flags.NewStandardFlagParser(
		flags.WithBoolFlag("recommended", "", false, "Show only recommended themes"),
		flags.WithEnvVars("recommended", "ATMOS_THEME_RECOMMENDED"),
	)

	// Register flags with cobra.
	themeListParser.RegisterFlags(themeListCmd)

	// Bind both env vars and pflags to viper for full precedence support (flag > env > config > default).
	if err := themeListParser.BindFlagsToViper(themeListCmd, viper.GetViper()); err != nil {
		// Log error but don't fail initialization.
		// This allows the command to still work even if Viper binding fails.
		_ = err
	}

	themeCmd.AddCommand(themeListCmd)
}

// executeThemeList runs the theme list command.
func executeThemeList(cmd *cobra.Command, args []string) error {
	defer perf.Track(atmosConfigPtr, "theme.list.RunE")()

	// Parse flags into options using Viper for proper precedence (flag > env > config > default).
	opts := &ThemeListOptions{
		RecommendedOnly: viper.GetBool("recommended"),
	}

	// Get the current active theme from configuration.
	activeTheme := ""
	if atmosConfigPtr != nil {
		activeTheme = atmosConfigPtr.Settings.Terminal.Theme
	}

	result := theme.ListThemes(theme.ListThemesOptions{
		RecommendedOnly: opts.RecommendedOnly,
		ActiveTheme:     activeTheme,
	})

	if result.Error != nil {
		return result.Error
	}

	// Write the table output.
	if err := ui.Write(result.Output); err != nil {
		return err
	}

	// Write footer messages with styling.
	countMsg := fmt.Sprintf("%d theme", result.ThemeCount)
	if result.ThemeCount != 1 {
		countMsg += "s"
	}

	if result.RecommendedOnly {
		countMsg += " (recommended). Use without --recommended to see all themes."
	} else {
		countMsg += " available."
		if result.ShowStars {
			countMsg += " " + theme.IconRecommended + " indicates recommended themes."
		}
	}

	if err := ui.Info(countMsg); err != nil {
		return err
	}

	if result.ActiveTheme != "" {
		if err := ui.Success(fmt.Sprintf("Active theme: %s", result.ActiveTheme)); err != nil {
			return err
		}
	}

	return nil
}
