package cmd

import (
	"github.com/spf13/cobra"
)

// themeCmd manages terminal theme configuration.
var themeCmd = &cobra.Command{
	Use:   "theme",
	Short: "Manage terminal themes for Atmos CLI",
	Long:  "Configure and preview terminal themes that control the appearance of CLI output, tables, and markdown rendering.",
	Example: `# List all available themes
atmos theme list

# List only recommended themes
atmos theme list --recommended

# Show details and preview a specific theme
atmos theme show dracula

# Set the active theme (updates configuration)
atmos theme set dracula`,
}

func init() {
	RootCmd.AddCommand(themeCmd)
}
