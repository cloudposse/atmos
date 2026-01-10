package cmd

import (
	"context"
	"os"
	"path/filepath"

	"github.com/charmbracelet/fang"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

// Execute runs the root command with Fang styling.
func Execute() error {
	// Load .env files (silently ignore if not present).
	// Priority: current directory, then demos directory.
	loadEnvFiles()

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

	// Add subcommands.
	root.AddCommand(newCmd())
	root.AddCommand(renderCmd())
	root.AddCommand(publishCmd())
	root.AddCommand(validateCmd())
	root.AddCommand(listCmd())    // Parent command for list subcommands.
	root.AddCommand(catalogCmd()) // Hidden, backward compat.
	root.AddCommand(showCmd())
	root.AddCommand(installCmd())
	root.AddCommand(exportCmd())
	root.AddCommand(migrateCmd())
	root.AddCommand(updateDurationCmd())
	root.AddCommand(updateThumbnailsCmd())

	// Execute with Fang enhancements (styled help, errors, etc.)
	return fang.Execute(
		context.Background(),
		root,
		fang.WithNotifySignal(os.Interrupt), // Handle Ctrl+C gracefully
	)
}

// loadEnvFiles loads .env files from standard locations.
// Files are loaded in order of priority (later files override earlier ones):
// 1. Current working directory (.env)
// 2. Demos directory (demos/.env)
// Errors are silently ignored - .env files are optional.
func loadEnvFiles() {
	// Load from current directory.
	_ = godotenv.Load()

	// Try to find and load from demos directory.
	if demosDir, err := findDemosDir(); err == nil {
		envFile := filepath.Join(demosDir, ".env")
		_ = godotenv.Load(envFile)
	}
}
