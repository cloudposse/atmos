package theme

import (
	_ "embed"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

//go:embed markdown/atmos_theme_show_usage.md
var themeShowUsage string

// themeShowParser is the flag parser for theme show command.
var themeShowParser *flags.StandardFlagParser

// ThemeShowOptions holds the options for theme show command.
type ThemeShowOptions struct {
	ThemeName string
}

// themeShowCmd shows details and preview of a specific theme.
var themeShowCmd = &cobra.Command{
	Use:     "show [theme-name]",
	Short:   "Show details and preview of a specific theme",
	Long:    "Display color palette and sample UI elements for a specific terminal theme.",
	Example: themeShowUsage,
	Args:    cobra.ExactArgs(1),
	RunE:    executeThemeShow,
}

func init() {
	// Create flag parser (no flags currently, but sets up the pattern).
	themeShowParser = flags.NewStandardFlagParser()

	// Register flags with cobra.
	themeShowParser.RegisterFlags(themeShowCmd)

	// Bind both env vars and pflags to viper for full precedence support (flag > env > config > default).
	if err := themeShowParser.BindFlagsToViper(themeShowCmd, viper.GetViper()); err != nil {
		// Log error but don't fail initialization.
		// This allows the command to still work even if Viper binding fails.
		_ = err
	}

	themeCmd.AddCommand(themeShowCmd)
}

// executeThemeShow displays detailed information about a specific theme.
//
// Note: This command is a preview/demo that shows what theme styles look like.
// The ui.Write() call correctly outputs the preview to the UI channel (stderr).
func executeThemeShow(cmd *cobra.Command, args []string) error {
	defer perf.Track(atmosConfigPtr, "theme.show.RunE")()

	// Parse command arguments into options.
	opts := &ThemeShowOptions{
		ThemeName: args[0],
	}

	result := theme.ShowTheme(theme.ShowThemeOptions{
		ThemeName: opts.ThemeName,
	})

	if result.Error != nil {
		return result.Error
	}

	return ui.Write(result.Output)
}
