package theme

import (
	_ "embed"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

//go:embed markdown/atmos_theme_list_usage.md
var themeListUsage string

var themeListRecommendedOnly bool

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
	themeListCmd.Flags().BoolVar(&themeListRecommendedOnly, "recommended", false, "Show only recommended themes")
	themeCmd.AddCommand(themeListCmd)
}

// executeThemeList runs the theme list command.
func executeThemeList(cmd *cobra.Command, args []string) error {
	defer perf.Track(atmosConfigPtr, "theme.list.RunE")()

	// Get the current active theme from configuration.
	activeTheme := ""
	if atmosConfigPtr != nil {
		activeTheme = atmosConfigPtr.Settings.Terminal.Theme
	}

	result := theme.ListThemes(theme.ListThemesOptions{
		RecommendedOnly: themeListRecommendedOnly,
		ActiveTheme:     activeTheme,
	})

	if result.Error != nil {
		return result.Error
	}

	return ui.Write(result.Output)
}
