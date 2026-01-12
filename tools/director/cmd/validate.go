package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/tools/director/internal/ffmpeg"
	"github.com/cloudposse/atmos/tools/director/internal/scene"
	"github.com/cloudposse/atmos/tools/director/internal/toolmgr"
	"github.com/cloudposse/atmos/tools/director/internal/validation"
	"github.com/cloudposse/atmos/tools/director/internal/vhs"
)

func validateCmd() *cobra.Command {
	var (
		rendered      bool
		tapes         bool
		all           bool
		category      string
		tag           string
		includeDrafts bool
	)

	cmd := &cobra.Command{
		Use:   "validate [scene-names...]",
		Short: "Validate scenes and rendered outputs",
		Long: `Validate all scenes defined in scenes.yaml.

Default validation checks:
- Tape files exist
- Audio files exist (if configured)
- Required dependencies are installed (atmos, terraform, etc.)
- FFmpeg is available (if any scene uses audio)

With --tapes flag:
- Validates tape file syntax using VHS validate
- Catches parsing errors before rendering (e.g., unquoted paths)

With --rendered flag:
- Validates rendered SVG outputs against error patterns
- Checks must_not_match patterns (e.g., "Error: ")
- Checks must_match patterns (e.g., expected output)`,
		Example: `
# Validate all enabled scenes configuration
director validate

# Validate tape file syntax for all enabled scenes
director validate --tapes

# Validate tapes for specific scenes
director validate terraform-plan describe-stacks --tapes

# Validate tapes by tag
director validate --tag featured --tapes

# Validate tapes by category
director validate --category terraform --tapes

# Validate all scenes (including disabled)
director validate --all --tapes

# Validate rendered SVG outputs for errors
director validate --rendered
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			demosDir, err := findDemosDir()
			if err != nil {
				return err
			}

			scenesFile := filepath.Join(demosDir, "scenes.yaml")
			scenesList, err := scene.LoadScenes(scenesFile)
			if err != nil {
				return fmt.Errorf("failed to load scenes: %w", err)
			}

			// If --rendered flag is set, validate SVG outputs.
			if rendered {
				return validateRenderedOutputs(cmd, demosDir, scenesList)
			}

			// If --tapes flag is set, validate tape file syntax.
			if tapes {
				// Filter scenes using the same logic as render command.
				scenesToValidate := filterScenes(scenesList, args, tag, category, all, includeDrafts)
				return validateTapeSyntax(ctx, cmd, demosDir, scenesToValidate)
			}

			// Otherwise, run the existing scene configuration validation.
			return validateSceneConfig(ctx, cmd, demosDir, scenesList)
		},
	}

	cmd.Flags().BoolVar(&rendered, "rendered", false,
		"Validate rendered SVG outputs for errors (checks for Error:, command not found, etc.)")
	cmd.Flags().BoolVar(&tapes, "tapes", false,
		"Validate tape file syntax using VHS validate (catches parsing errors)")
	cmd.Flags().BoolVar(&all, "all", false,
		"Include all scenes (including disabled) - use with --tapes")
	cmd.Flags().StringVarP(&category, "category", "c", "",
		"Filter by gallery category (e.g., terraform, list) - use with --tapes")
	cmd.Flags().StringVarP(&tag, "tag", "t", "",
		"Filter by tag (e.g., featured, version) - use with --tapes")
	cmd.Flags().BoolVar(&includeDrafts, "include-drafts", false,
		"Include draft scenes (status: draft) - use with --tapes")

	return cmd
}

// filterScenes filters scenes based on args, tag, category, and other flags.
// This mirrors the filtering logic in render command.
func filterScenes(scenesList *scene.ScenesList, args []string, tag, category string, all, includeDrafts bool) []*scene.Scene {
	var filtered []*scene.Scene

	if len(args) > 0 {
		// Filter by specific scene names.
		requestedNames := make(map[string]bool)
		for _, name := range args {
			requestedNames[name] = true
		}
		for _, sc := range scenesList.Scenes {
			if requestedNames[sc.Name] {
				filtered = append(filtered, sc)
			}
		}
	} else if tag != "" {
		// Filter by tag.
		for _, sc := range scenesList.Scenes {
			if sc.IsDraft() && !includeDrafts {
				continue
			}
			if sc.HasTag(tag) && (all || sc.Enabled) {
				filtered = append(filtered, sc)
			}
		}
	} else if category != "" {
		// Filter by category.
		for _, sc := range scenesList.Scenes {
			if sc.IsDraft() && !includeDrafts {
				continue
			}
			if sc.GetCategory() == category && (all || sc.Enabled) {
				filtered = append(filtered, sc)
			}
		}
	} else {
		// All enabled (or all if --all flag).
		for _, sc := range scenesList.Scenes {
			if sc.IsDraft() && !includeDrafts {
				continue
			}
			if all || sc.Enabled {
				filtered = append(filtered, sc)
			}
		}
	}

	return filtered
}

// validateTapeSyntax validates tape file syntax using VHS validate.
func validateTapeSyntax(ctx context.Context, cmd *cobra.Command, demosDir string, scenes []*scene.Scene) error {
	// Check VHS is installed.
	if err := vhs.CheckInstalled(); err != nil {
		return err
	}

	cmd.Printf("Validating tape syntax for %d scene(s)...\n\n", len(scenes))

	hasErrors := false
	passCount := 0

	for _, sc := range scenes {
		tapeFile := filepath.Join(demosDir, sc.Tape)

		// Check tape file exists first.
		if _, err := os.Stat(tapeFile); os.IsNotExist(err) {
			cmd.Printf("✗ %s: tape file not found: %s\n", sc.Name, sc.Tape)
			hasErrors = true
			continue
		}

		// Resolve workdir the same way as render does.
		workdir := demosDir
		if sc.Workdir != "" {
			workdir = filepath.Join(filepath.Dir(demosDir), sc.Workdir)
		}

		// Preprocess tape to inline Source directives.
		tempTape, err := vhs.PreprocessTape(tapeFile)
		if err != nil {
			cmd.Printf("✗ %s: failed to preprocess tape: %v\n", sc.Name, err)
			hasErrors = true
			continue
		}

		// Validate preprocessed tape syntax from workdir.
		if err := vhs.ValidateTape(ctx, tempTape, workdir); err != nil {
			os.Remove(tempTape)
			cmd.Printf("✗ %s\n", sc.Name)
			// Indent the error output.
			lines := strings.Split(err.Error(), "\n")
			for _, line := range lines {
				if line != "" {
					cmd.Printf("    %s\n", line)
				}
			}
			hasErrors = true
		} else {
			os.Remove(tempTape)
			cmd.Printf("✓ %s\n", sc.Name)
			passCount++
		}
	}

	cmd.Println()
	cmd.Printf("Passed: %d/%d\n", passCount, len(scenes))

	if hasErrors {
		return fmt.Errorf("tape validation failed")
	}

	cmd.Println("All tapes validated successfully!")
	return nil
}

// validateSceneConfig validates scene configuration (tape files, audio, dependencies).
func validateSceneConfig(ctx context.Context, cmd *cobra.Command, demosDir string, scenesList *scene.ScenesList) error {
	// Load and display tools configuration.
	toolsConfig, err := toolmgr.LoadToolsConfig(demosDir)
	if err != nil {
		return fmt.Errorf("failed to load tools config: %w", err)
	}

	if toolsConfig != nil && toolsConfig.Atmos != nil {
		cmd.Println("Tool versions:")
		mgr := toolmgr.New(toolsConfig, demosDir)

		// Check if atmos is installed at correct version.
		path, err := mgr.EnsureInstalled(ctx, "atmos")
		if err != nil {
			cmd.Printf("  ✗ atmos: %v\n", err)
		} else {
			version, _ := mgr.Version("atmos")
			cmd.Printf("  ✓ atmos v%s (%s)\n", version, path)
		}
		cmd.Println()
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
			return fmt.Errorf("ffmpeg validation failed: %w", err)
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
}

// validateRenderedOutputs validates rendered SVG files against error patterns.
func validateRenderedOutputs(cmd *cobra.Command, demosDir string, scenesList *scene.ScenesList) error {
	// Load defaults configuration for validation rules.
	defaults, _ := toolmgr.LoadDefaultsConfig(demosDir)
	var validationDefaults *scene.ValidationConfig
	if defaults != nil && defaults.Validation != nil {
		validationDefaults = defaults.Validation
	}

	validator := validation.New(validationDefaults)

	cmd.Printf("Validating rendered outputs for %d scene(s)...\n\n", len(scenesList.Scenes))

	hasErrors := false
	validatedCount := 0
	skippedCount := 0

	for _, sc := range scenesList.Scenes {
		if !sc.Enabled {
			cmd.Printf("⊘ %s (disabled)\n", sc.Name)
			skippedCount++
			continue
		}

		// Check if SVG is in outputs.
		hasSVG := false
		for _, output := range sc.Outputs {
			if output == "svg" {
				hasSVG = true
				break
			}
		}
		if !hasSVG {
			cmd.Printf("⊘ %s (no SVG output configured)\n", sc.Name)
			skippedCount++
			continue
		}

		// Find the SVG output file.
		svgPath := findSVGOutput(demosDir, sc)
		if svgPath == "" {
			cmd.Printf("⊘ %s (SVG not rendered yet)\n", sc.Name)
			skippedCount++
			continue
		}

		// Validate the SVG.
		result, err := validator.ValidateSVG(svgPath, sc.Validate)
		if err != nil {
			cmd.Printf("✗ %s: %v\n", sc.Name, err)
			hasErrors = true
			continue
		}

		validatedCount++

		if result.Passed {
			cmd.Printf("✓ %s\n", sc.Name)
		} else {
			cmd.Printf("✗ %s\n", sc.Name)
			for _, e := range result.Errors {
				cmd.Printf("    %s\n", e)
			}
			for _, m := range result.Missing {
				cmd.Printf("    %s\n", m)
			}
			hasErrors = true
		}
	}

	cmd.Println()
	cmd.Printf("Validated: %d, Skipped: %d\n", validatedCount, skippedCount)

	if hasErrors {
		return fmt.Errorf("validation failed")
	}

	if validatedCount > 0 {
		cmd.Println("All rendered outputs validated successfully!")
	} else {
		cmd.Println("No SVG outputs found to validate. Run 'director render' first.")
	}
	return nil
}

// findSVGOutput locates the SVG output file for a scene.
func findSVGOutput(demosDir string, sc *scene.Scene) string {
	// SVG is typically output alongside the tape file.
	// The output name is derived from the tape file or scene name.
	tapeDir := filepath.Dir(filepath.Join(demosDir, sc.Tape))
	sceneName := sc.Name

	// Try common output locations.
	candidates := []string{
		filepath.Join(demosDir, ".cache", sceneName+".svg"),
		filepath.Join(tapeDir, sceneName+".svg"),
		filepath.Join(demosDir, "output", sceneName+".svg"),
		filepath.Join(demosDir, "renders", sceneName+".svg"),
	}

	// Also try using the tape filename as the base.
	tapeBase := strings.TrimSuffix(filepath.Base(sc.Tape), ".tape")
	candidates = append(candidates,
		filepath.Join(tapeDir, tapeBase+".svg"),
	)

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}
