package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/tools/director/internal/scene"
)

func catalogCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "catalog",
		Short: "List all demo scenes",
		Long: `List all demo scenes defined in scenes.yaml.

Shows scene names, descriptions, requirements, and output formats.
Disabled scenes are marked with ⊘.`,
		Example: `
# List all scenes
director catalog

# Shows a formatted catalog with all scene metadata
`,
		Aliases: []string{"list", "ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			demosDir, err := findDemosDir()
			if err != nil {
				return err
			}

			scenesFile := filepath.Join(demosDir, "scenes.yaml")
			scenesList, err := scene.LoadScenes(scenesFile)
			if err != nil {
				return fmt.Errorf("failed to load scenes: %w", err)
			}

			cmd.Printf("Atmos Demo Scenes (%d total)\n\n", len(scenesList.Scenes))

			for _, sc := range scenesList.Scenes {
				status := "✓"
				if !sc.Enabled {
					status = "⊘"
				}

				cmd.Printf("%s %-30s %s\n", status, sc.Name, sc.Description)
				cmd.Printf("  Tape: %s\n", sc.Tape)
				if len(sc.Requires) > 0 {
					cmd.Printf("  Requires: %v\n", sc.Requires)
				}
				if len(sc.Outputs) > 0 {
					cmd.Printf("  Outputs: %v\n", sc.Outputs)
				}
				cmd.Println()
			}

			return nil
		},
	}
}
