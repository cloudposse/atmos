package cmd

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// themeListCmd lists available terminal themes.
var themeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available terminal themes",
	Long:  "Display available terminal themes that can be used for markdown rendering. By default shows all themes.",
	Example: `# List all themes
atmos theme list

# Show only recommended themes
atmos theme list --recommended`,
	Args: cobra.NoArgs,
	RunE: executeThemeList,
}

var themeListRecommendedOnly bool

func init() {
	themeListCmd.Flags().BoolVar(&themeListRecommendedOnly, "recommended", false, "Show only recommended themes")
	themeCmd.AddCommand(themeListCmd)
}

// executeThemeList runs the theme list command.
func executeThemeList(cmd *cobra.Command, args []string) error {
	// Get the current active theme from configuration.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, false)
	activeTheme := ""
	if err == nil {
		activeTheme = atmosConfig.Settings.Terminal.Theme
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
