package cmd

import (
	"context"
	"os"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"
)

// Execute runs the root command with Fang styling.
func Execute() error {
	root := &cobra.Command{
		Use:   "director",
		Short: "Demo scene management for Atmos VHS videos",
		Long: `Director manages demo scenes for VHS rendering.

It handles scene creation, validation, rendering, and catalog management
for Atmos demonstration videos and screenshots.`,
		Example: `
# Build director
make director

# Validate all scenes
director validate

# Render all scenes
director render

# Create new scene
director new terraform-apply-basic
`,
	}

	// Add subcommands
	root.AddCommand(newCmd())
	root.AddCommand(renderCmd())
	root.AddCommand(validateCmd())
	root.AddCommand(catalogCmd())
	root.AddCommand(showCmd())

	// Execute with Fang enhancements (styled help, errors, etc.)
	return fang.Execute(
		context.Background(),
		root,
		fang.WithNotifySignal(os.Interrupt), // Handle Ctrl+C gracefully
	)
}
