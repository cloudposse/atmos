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

	// Bind env vars to Viper during init for environment variable support.
	if err := themeListParser.BindToViper(viper.GetViper()); err != nil {
		// Log error but don't fail initialization.
		// This allows the command to still work even if Viper binding fails.
		_ = err
	}

	themeCmd.AddCommand(themeListCmd)
}

// executeThemeList runs the theme list command.
func executeThemeList(cmd *cobra.Command, args []string) error {
	defer perf.Track(atmosConfigPtr, "theme.list.RunE")()

	// Bind parsed flags to Viper for precedence (flag > env > config > default).
	v := viper.GetViper()
	if err := themeListParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Parse flags into options using Viper for proper precedence.
	opts := &ThemeListOptions{
		RecommendedOnly: v.GetBool("recommended"),
	}

	// Get the current active theme from configuration.
	// Use the same resolution logic as the theme system itself:
	// 1. Check atmos.yaml config
	// 2. Check ATMOS_THEME env var
	// 3. Check THEME env var
	// 4. Default to "atmos"
	activeTheme := ""
	if atmosConfigPtr != nil && atmosConfigPtr.Settings.Terminal.Theme != "" {
		activeTheme = atmosConfigPtr.Settings.Terminal.Theme
	} else if envTheme := v.GetString("ATMOS_THEME"); envTheme != "" {
		activeTheme = envTheme
	} else if envTheme := v.GetString("THEME"); envTheme != "" {
		activeTheme = envTheme
	} else {
		// Default to "atmos" theme (same as pkg/ui/theme/styles.go:getActiveThemeName)
		activeTheme = "atmos"
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
