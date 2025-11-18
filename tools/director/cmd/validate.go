package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/ffmpeg"
	"github.com/cloudposse/atmos/tools/director/internal/scene"
)

func validateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate all scenes",
		Long: `Validate all scenes defined in scenes.yaml.

Checks:
- Tape files exist
- Audio files exist (if configured)
- Required dependencies are installed (atmos, terraform, etc.)
- FFmpeg is available (if any scene uses audio)
- VHS can parse the tape files`,
		Example: `
# Validate all scenes
director validate

# Shows which scenes are enabled/disabled and any errors
`,
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

			cmd.Printf("Validating %d scene(s)...\n\n", len(scenesList.Scenes))

			// Check if any scene uses audio and needs FFmpeg.
			needsFFmpeg := false
			for _, sc := range scenesList.Scenes {
				if sc.Enabled && sc.Audio != nil && sc.Audio.Source != "" {
					needsFFmpeg = true
					break
				}
			}

			// Validate FFmpeg if needed.
			if needsFFmpeg {
				if err := ffmpeg.CheckInstalled(); err != nil {
					cmd.Printf("✗ FFmpeg validation failed: %v\n", err)
					cmd.Println("  Required for scenes with background audio")
					return fmt.Errorf("ffmpeg validation failed")
				}
			}

			hasErrors := false
			for _, sc := range scenesList.Scenes {
				if !sc.Enabled {
					cmd.Printf("⊘ %s (disabled)\n", sc.Name)
					continue
				}

				// Check if tape file exists.
				tapeFile := filepath.Join(demosDir, sc.Tape)
				if _, err := os.Stat(tapeFile); os.IsNotExist(err) {
					cmd.Printf("✗ %s: tape file not found: %s\n", sc.Name, sc.Tape)
					hasErrors = true
					continue
				}

				// Check if audio file exists (if configured).
				if sc.Audio != nil && sc.Audio.Source != "" {
					audioFile := filepath.Join(demosDir, sc.Audio.Source)
					if _, err := os.Stat(audioFile); os.IsNotExist(err) {
						cmd.Printf("✗ %s: audio file not found: %s\n", sc.Name, sc.Audio.Source)
						hasErrors = true
						continue
					}

					// Validate MP4 is in outputs if audio is configured.
					hasMP4 := false
					for _, output := range sc.Outputs {
						if output == "mp4" {
							hasMP4 = true
							break
						}
					}
					if !hasMP4 {
						cmd.Printf("✗ %s: audio configured but 'mp4' not in outputs\n", sc.Name)
						hasErrors = true
						continue
					}
				}

				// Check dependencies.
				missingDeps := []string{}
				for _, dep := range sc.Requires {
					if _, err := exec.LookPath(dep); err != nil {
						missingDeps = append(missingDeps, dep)
					}
				}

				if len(missingDeps) > 0 {
					cmd.Printf("✗ %s: missing dependencies: %v\n", sc.Name, missingDeps)
					hasErrors = true
				} else {
					cmd.Printf("✓ %s\n", sc.Name)
				}
			}

			cmd.Println()
			if hasErrors {
				return fmt.Errorf("validation failed")
			}

			cmd.Println("All scenes validated successfully!")
			return nil
		},
	}
}
