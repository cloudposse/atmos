package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/ffmpeg"
	"github.com/cloudposse/atmos/pkg/vhs"
	"github.com/cloudposse/atmos/tools/director/internal/scene"
	vhsRenderer "github.com/cloudposse/atmos/tools/director/internal/vhs"
)

func renderCmd() *cobra.Command {
	var (
		all   bool
		force bool
	)

	cmd := &cobra.Command{
		Use:   "render [scene-names...]",
		Short: "Render demo scenes to GIF/PNG",
		Long: `Render one or more scenes using VHS.

By default, renders all enabled scenes from scenes.yaml.
Specify scene names to render only those scenes.

Uses incremental rendering - only regenerates scenes if the tape file
has changed (unless --force is specified).`,
		Example: `
# Render all enabled scenes
director render

# Render specific scenes
director render terraform-plan-basic describe-stacks

# Render all scenes (including disabled)
director render --all

# Force re-render even if outputs exist
director render --force terraform-plan-basic
`,
		RunE: func(c *cobra.Command, args []string) error {
			demosDir, err := findDemosDir()
			if err != nil {
				return err
			}

			scenesFile := filepath.Join(demosDir, "scenes.yaml")
			scenesList, err := scene.LoadScenes(scenesFile)
			if err != nil {
				return fmt.Errorf("failed to load scenes: %w", err)
			}

			// Filter scenes to render.
			var scenesToRender []*scene.Scene
			if len(args) > 0 {
				// Render specific scenes.
				requestedNames := make(map[string]bool)
				for _, name := range args {
					requestedNames[name] = true
				}

				for _, sc := range scenesList.Scenes {
					if requestedNames[sc.Name] {
						scenesToRender = append(scenesToRender, sc)
					}
				}

				if len(scenesToRender) == 0 {
					return fmt.Errorf("no matching scenes found for: %v", args)
				}
			} else {
				// Render all scenes (or all enabled).
				for _, sc := range scenesList.Scenes {
					if all || sc.Enabled {
						scenesToRender = append(scenesToRender, sc)
					}
				}
			}

			// Check if VHS is installed before attempting any renders.
			if err := vhs.CheckInstalled(); err != nil {
				return err
			}

			// Check if any scene uses audio and needs FFmpeg.
			needsFFmpeg := false
			for _, sc := range scenesToRender {
				if sc.Audio != nil && sc.Audio.Source != "" {
					needsFFmpeg = true
					break
				}
			}

			// Check FFmpeg availability if needed.
			if needsFFmpeg {
				if err := ffmpeg.CheckInstalled(); err != nil {
					return err
				}
			}

			c.Printf("Rendering %d scene(s)...\n\n", len(scenesToRender))

			renderer := vhsRenderer.NewRenderer(demosDir)
			renderer.SetForce(force)

			ctx := context.Background()
			successCount := 0
			for _, sc := range scenesToRender {
				c.Printf("Rendering %s... ", sc.Name)

				if err := renderer.Render(ctx, sc); err != nil {
					c.Printf("FAILED\n  Error: %v\n", err)
					continue
				}

				c.Println("OK")
				successCount++
			}

			c.Printf("\nRendered %d/%d scene(s) successfully\n", successCount, len(scenesToRender))

			if successCount < len(scenesToRender) {
				return fmt.Errorf("some scenes failed to render")
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Render all scenes (including disabled)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force re-render (ignore cache)")

	return cmd
}
