package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/tools/director/internal/scene"
)

func showCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "show <scene-name>",
		Aliases: []string{"view", "open"},
		Short:   "Open a scene's tape file for viewing",
		Long: `Open a scene's tape file in the default editor or viewer.

Uses the system's default application for .tape files (typically a text editor).`,
		Example: `
# Open a scene's tape file
director show demo-stacks-describe

# Also works with aliases
director view demo-stacks-describe
director open demo-stacks-describe
`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			sceneName := args[0]

			// Determine demos directory.
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get working directory: %w", err)
			}

			demosDir := cwd
			if filepath.Base(cwd) != "demos" {
				demosDir = filepath.Join(cwd, "demos")
			}

			// Load scenes.yaml.
			scenesFile := filepath.Join(demosDir, "scenes.yaml")
			data, err := os.ReadFile(scenesFile)
			if err != nil {
				return fmt.Errorf("failed to read scenes.yaml: %w", err)
			}

			var scenesList scene.ScenesList
			if err := yaml.Unmarshal(data, &scenesList); err != nil {
				return fmt.Errorf("failed to parse scenes.yaml: %w", err)
			}

			// Find the scene.
			var targetScene *scene.Scene
			for _, sc := range scenesList.Scenes {
				if sc.Name == sceneName {
					targetScene = sc
					break
				}
			}

			if targetScene == nil {
				return fmt.Errorf("scene not found: %s", sceneName)
			}

			// Build tape file path.
			tapeFile := filepath.Join(demosDir, targetScene.Tape)

			// Check if file exists.
			if _, err := os.Stat(tapeFile); os.IsNotExist(err) {
				return fmt.Errorf("tape file not found: %s", tapeFile)
			}

			// Open the file with system default application.
			if err := openFile(tapeFile); err != nil {
				return fmt.Errorf("failed to open file: %w", err)
			}

			c.Printf("Opened %s\n", tapeFile)
			return nil
		},
	}
}

// openFile opens a file with the system's default application.
func openFile(path string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", path)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Start()
}
