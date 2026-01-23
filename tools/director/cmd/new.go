package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "new <scene-name>",
		Short: "Create a new demo scene",
		Long: `Create a new demo scene with a template VHS tape file.

Scene names must be in kebab-case (lowercase with hyphens).
The scene will be created in demos/scenes/ with a .tape extension.`,
		Example: `
# Create a new scene for terraform plan
director new terraform-plan-basic

# Create a scene for workflow execution
director new workflow-deploy-prod

# Create a scene in a subfolder (must exist)
director new demo-stacks/new-feature
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sceneName := args[0]

			// Validate scene name (kebab-case)
			if !isValidSceneName(sceneName) {
				return fmt.Errorf("invalid scene name %q: use kebab-case (e.g., terraform-plan-basic)", sceneName)
			}

			// Find demos directory
			demosDir, err := findDemosDir()
			if err != nil {
				return err
			}

			scenesDir := filepath.Join(demosDir, "scenes")
			tapeFile := filepath.Join(scenesDir, sceneName+".tape")

			// Check if scene already exists.
			if _, err := os.Stat(tapeFile); err == nil {
				return fmt.Errorf("scene %q already exists at %s", sceneName, tapeFile)
			}

			// Ensure parent directory exists (for subfolder scenes)
			if err := os.MkdirAll(filepath.Dir(tapeFile), 0o755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

			// Create tape file from template.
			template := fmt.Sprintf(`# %s scene
Output %s.gif

# VHS Configuration (from demos/defaults.yaml)
Set Theme "Catppuccin Mocha"
Set Width 1400
Set Height 800
Set FontSize 14
Set TypingSpeed 50ms

# Dependencies (checked by director validate)
Require atmos

# Setup
Type "export ATMOS_BASE_PATH=examples/complete"
Enter
Sleep 500ms

# Execute command
Type "atmos --help"
Sleep 500ms
Enter
Sleep 2s

# Capture screenshot
Screenshot %s.png

# Cleanup
Sleep 1s
`, sceneName, sceneName, sceneName)

			if err := os.WriteFile(tapeFile, []byte(template), 0o644); err != nil {
				return fmt.Errorf("failed to create tape file: %w", err)
			}

			cmd.Printf("Created scene: %s\n", tapeFile)
			cmd.Println("\nNext steps:")
			cmd.Printf("1. Edit the tape file: %s\n", tapeFile)
			cmd.Println("2. Add scene to demos/scenes.yaml")
			cmd.Printf("3. Test: director render %s\n", sceneName)
			cmd.Printf("4. Commit: git add %s demos/scenes.yaml\n", tapeFile)

			return nil
		},
	}
}

func isValidSceneName(name string) bool {
	if len(name) == 0 {
		return false
	}
	// Allow slashes for subfolder scenes
	for _, ch := range name {
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '/') {
			return false
		}
	}
	return !strings.HasPrefix(name, "-") && !strings.HasSuffix(name, "-")
}

func findDemosDir() (string, error) {
	// Look for demos directory from current working directory up to root.
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir := cwd
	for {
		demosDir := filepath.Join(dir, "demos")
		if stat, err := os.Stat(demosDir); err == nil && stat.IsDir() {
			return demosDir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("demos directory not found (run from atmos repo root)")
}
