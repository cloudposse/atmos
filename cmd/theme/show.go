package theme

import (
	_ "embed"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

//go:embed markdown/atmos_theme_show_usage.md
var themeShowUsage string

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
	themeCmd.AddCommand(themeShowCmd)
}

// executeThemeShow displays detailed information about a specific theme.
//
// Note: This command is a preview/demo that shows what theme styles look like.
// The ui.Write() call correctly outputs the preview to the UI channel (stderr).
func executeThemeShow(cmd *cobra.Command, args []string) error {
	defer perf.Track(atmosConfigPtr, "theme.show.RunE")()

	result := theme.ShowTheme(theme.ShowThemeOptions{
		ThemeName: args[0],
	})

	if result.Error != nil {
		return result.Error
	}

	return ui.Write(result.Output)
}
