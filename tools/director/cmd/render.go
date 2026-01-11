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
	"github.com/cloudposse/atmos/tools/director/internal/tape"
	"github.com/cloudposse/atmos/tools/director/internal/toolmgr"
	"github.com/cloudposse/atmos/tools/director/internal/validation"
	"github.com/cloudposse/atmos/tools/director/internal/vhs"
)

func renderCmd() *cobra.Command {
	var (
		all            bool
		force          bool
		publish        bool
		exportManifest bool
		formats        string
		noSVGFix       bool
		category       string
		tag            string
		validate       bool
		includeDrafts  bool
		testMode       bool
		verbose        bool
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

# Render and publish in one command
director render list-vars --force --publish

# Render, publish, and export manifest
director render list-vars --force --publish --export-manifest

# Render only SVG format (faster for testing)
director render terraform-plan --format svg

# Render multiple specific formats
director render terraform-plan --format svg,gif

# Render all scenes in a category (uses gallery.category)
director render --category terraform --force --publish

# Render all scenes with a specific tag
director render --tag version --force --publish

# Render all featured scenes
director render --tag featured --force

# Render and publish all list-related demos
director render --category list --force --publish --export-manifest

# Render and validate outputs for errors
director render terraform-plan --force --validate

# Full pipeline: render, publish, export manifest, and validate
director render --category vendor --force --publish --export-manifest --validate

# Include draft scenes in rendering (not included by default)
director render --all --include-drafts

# Test mode: execute commands without rendering (fast validation)
director render --tag featured --test

# Test specific scene commands
director render terraform-plan --test
`,
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()

			demosDir, err := findDemosDir()
			if err != nil {
				return err
			}

			// Load full defaults configuration including hooks.
			defaultsConfig, err := toolmgr.LoadDefaultsConfig(demosDir)
			if err != nil {
				return fmt.Errorf("failed to load defaults config: %w", err)
			}

			// Run pre-render hooks before anything else.
			if defaultsConfig != nil && defaultsConfig.Hooks != nil && len(defaultsConfig.Hooks.PreRender) > 0 {
				repoRoot := filepath.Dir(demosDir)
				if err := runPreRenderHooks(ctx, c, defaultsConfig.Hooks.PreRender, repoRoot); err != nil {
					return fmt.Errorf("pre-render hooks failed: %w", err)
				}
			}

			// Load tools configuration and ensure atmos is installed at the correct version.
			var toolsConfig *toolmgr.ToolsConfig
			if defaultsConfig != nil {
				toolsConfig = defaultsConfig.Tools
			}

			if toolsConfig != nil && toolsConfig.Atmos != nil {
				mgr := toolmgr.New(toolsConfig, demosDir)
				atmosPath, err := mgr.EnsureInstalled(ctx, "atmos")
				if err != nil {
					return fmt.Errorf("failed to ensure atmos is installed: %w", err)
				}
				// Prepend install directory to PATH so VHS uses our version.
				toolmgr.PrependToPath(filepath.Dir(atmosPath))
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
			} else if tag != "" {
				// Render all scenes with a specific tag.
				for _, sc := range scenesList.Scenes {
					// Skip drafts unless --include-drafts is set.
					if sc.IsDraft() && !includeDrafts {
						continue
					}
					if sc.HasTag(tag) && (all || sc.Enabled) {
						scenesToRender = append(scenesToRender, sc)
					}
				}

				if len(scenesToRender) == 0 {
					return fmt.Errorf("no matching scenes found for tag: %s", tag)
				}
			} else if category != "" {
				// Render all scenes in a category (uses gallery.category).
				for _, sc := range scenesList.Scenes {
					// Skip drafts unless --include-drafts is set.
					if sc.IsDraft() && !includeDrafts {
						continue
					}
					if sc.GetCategory() == category && (all || sc.Enabled) {
						scenesToRender = append(scenesToRender, sc)
					}
				}

				if len(scenesToRender) == 0 {
					return fmt.Errorf("no matching scenes found for category: %s", category)
				}
			} else {
				// Render all scenes (or all enabled).
				for _, sc := range scenesList.Scenes {
					// Skip drafts unless --include-drafts is set.
					if sc.IsDraft() && !includeDrafts {
						continue
					}
					if all || sc.Enabled {
						scenesToRender = append(scenesToRender, sc)
					}
				}
			}

			// If test mode, run commands from tape files without rendering.
			// --verbose implies --test (no need to specify both).
			if testMode || verbose {
				return runTestMode(ctx, c, demosDir, scenesToRender, verbose)
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

			// Check if any scene needs SVG output (considering format filter).
			needsSVG := false
			for _, sc := range scenesToRender {
				for _, output := range sc.Outputs {
					if output == "svg" {
						// If format filter is set, only check if svg is in the filter.
						if formats == "" || strings.Contains(formats, "svg") {
							needsSVG = true
							break
						}
					}
				}
				if needsSVG {
					break
				}
			}

			// Check VHS SVG support if needed.
			if needsSVG {
				if err := vhs.CheckSVGSupport(); err != nil {
					return err
				}
			}

			// Parse format filter.
			var formatFilter []string
			if formats != "" {
				formatFilter = strings.Split(formats, ",")
				for i, f := range formatFilter {
					formatFilter[i] = strings.TrimSpace(f)
				}
			}

			c.Printf("Rendering %d scene(s)...\n\n", len(scenesToRender))

			renderer := vhs.NewRenderer(demosDir)
			renderer.SetForce(force)
			renderer.SetSkipSVGFix(noSVGFix)
			if len(formatFilter) > 0 {
				renderer.SetFormatFilter(formatFilter)
			}

			successCount := 0
			var renderedSceneNames []string
			for _, sc := range scenesToRender {
				c.Printf("Rendering %s... ", sc.Name)

				result, err := renderer.Render(ctx, sc)
				if err != nil {
					c.Printf("FAILED\n  Error: %v\n", err)
					continue
				}

				if result.Cached {
					c.Println("CACHED (use --force to re-render)")
				} else {
					c.Println("OK")
				}

				// Show output file paths (relative to demos dir).
				for _, path := range result.OutputPaths {
					relPath, err := filepath.Rel(demosDir, path)
					if err != nil {
						relPath = path // Fallback to absolute if rel fails.
					}
					c.Printf("  → %s\n", relPath)
				}

				successCount++
				renderedSceneNames = append(renderedSceneNames, sc.Name)
			}

			c.Printf("\nRendered %d/%d scene(s) successfully\n", successCount, len(scenesToRender))

			if successCount < len(scenesToRender) {
				return fmt.Errorf("some scenes failed to render")
			}

			// If --publish flag is set, publish the rendered scenes.
			if publish && len(renderedSceneNames) > 0 {
				c.Printf("\n")
				if err := runPublish(ctx, c, demosDir, renderedSceneNames, force, formatFilter); err != nil {
					return fmt.Errorf("publish failed: %w", err)
				}
			}

			// If --export-manifest flag is set, export the manifest.
			if exportManifest {
				c.Printf("\n")
				if err := runExportManifest(demosDir); err != nil {
					return fmt.Errorf("export manifest failed: %w", err)
				}
			}

			// If --validate flag is set, validate the rendered SVGs.
			if validate && len(renderedSceneNames) > 0 {
				c.Printf("\n")
				if err := runValidation(c, demosDir, scenesList, renderedSceneNames, defaultsConfig); err != nil {
					return fmt.Errorf("validation failed: %w", err)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Render all scenes (including disabled)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force re-render (ignore cache)")
	cmd.Flags().BoolVarP(&publish, "publish", "p", false, "Publish rendered scenes after rendering")
	cmd.Flags().BoolVarP(&exportManifest, "export-manifest", "e", false, "Export manifest after publishing")
	cmd.Flags().StringVar(&formats, "format", "", "Only render specific formats (comma-separated: svg,gif,mp4,png)")
	cmd.Flags().BoolVar(&noSVGFix, "no-svg-fix", false, "Skip SVG line-height post-processing")
	cmd.Flags().StringVarP(&category, "category", "c", "", "Render all scenes in a gallery category (e.g., terraform, list, dx)")
	cmd.Flags().StringVarP(&tag, "tag", "t", "", "Render all scenes with a specific tag (e.g., version, featured)")
	cmd.Flags().BoolVar(&validate, "validate", false, "Validate rendered SVG outputs after rendering")
	cmd.Flags().BoolVar(&includeDrafts, "include-drafts", false, "Include draft scenes (status: draft) in rendering")
	cmd.Flags().BoolVarP(&testMode, "test", "T", false, "Test mode: execute commands from tape files without rendering")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Test mode with output: execute commands and show results (implies --test)")

	return cmd
}

// runPreRenderHooks runs pre-render hook commands before VHS rendering.
func runPreRenderHooks(ctx context.Context, c *cobra.Command, hooks []string, workdir string) error {
	c.Printf("Running pre-render hooks...\n")

	for i, cmdStr := range hooks {
		c.Printf("  [%d/%d] %s\n", i+1, len(hooks), cmdStr)

		cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
		cmd.Dir = workdir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("hook %d failed (%s): %w", i+1, cmdStr, err)
		}
	}

	c.Printf("Pre-render hooks completed.\n\n")
	return nil
}

// runValidation validates rendered SVG outputs for the given scenes.
func runValidation(c *cobra.Command, demosDir string, scenesList *scene.ScenesList, renderedSceneNames []string, defaults *toolmgr.DefaultsConfig) error {
	// Build a set of rendered scene names for quick lookup.
	renderedSet := make(map[string]bool)
	for _, name := range renderedSceneNames {
		renderedSet[name] = true
	}

	// Get validation defaults.
	var validationDefaults *scene.ValidationConfig
	if defaults != nil && defaults.Validation != nil {
		validationDefaults = defaults.Validation
	}

	validator := validation.New(validationDefaults)

	c.Printf("Validating rendered SVG outputs...\n\n")

	hasErrors := false
	validatedCount := 0

	for _, sc := range scenesList.Scenes {
		// Only validate scenes that were just rendered.
		if !renderedSet[sc.Name] {
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
			continue
		}

		// Find the SVG output file.
		svgPath := findSVGOutput(demosDir, sc)
		if svgPath == "" {
			c.Printf("⊘ %s (SVG not found)\n", sc.Name)
			continue
		}

		// Validate the SVG.
		result, err := validator.ValidateSVG(svgPath, sc.Validate)
		if err != nil {
			c.Printf("✗ %s: %v\n", sc.Name, err)
			hasErrors = true
			continue
		}

		validatedCount++

		if result.Passed {
			c.Printf("✓ %s\n", sc.Name)
		} else {
			c.Printf("✗ %s\n", sc.Name)
			for _, e := range result.Errors {
				c.Printf("    %s\n", e)
			}
			for _, m := range result.Missing {
				c.Printf("    %s\n", m)
			}
			hasErrors = true
		}
	}

	c.Printf("\nValidated %d scene(s)\n", validatedCount)

	if hasErrors {
		return fmt.Errorf("some scenes failed validation")
	}

	return nil
}

// runTestMode executes commands from tape files without rendering.
// This is useful for validating that demo commands work correctly.
// Exits on first failure (fail-fast behavior).
// If verbose is true, shows command output even on success.
func runTestMode(ctx context.Context, c *cobra.Command, demosDir string, scenes []*scene.Scene, verbose bool) error {
	c.Printf("Testing %d scene(s)...\n\n", len(scenes))

	for _, sc := range scenes {
		tapePath := filepath.Join(demosDir, sc.Tape)

		// Resolve workdir.
		workdir := demosDir
		if sc.Workdir != "" {
			workdir = filepath.Join(filepath.Dir(demosDir), sc.Workdir)
		}

		c.Printf("Testing scene: %s\n", sc.Name)
		c.Printf("Workdir: %s\n\n", workdir)

		// Run prep commands if any.
		if len(sc.Prep) > 0 {
			for _, prep := range sc.Prep {
				prepCmd := exec.CommandContext(ctx, "bash", "-c", prep)
				prepCmd.Dir = workdir
				if err := prepCmd.Run(); err != nil {
					c.Printf("  ✗ prep: %s\n", prep)
					c.Printf("    Error: %v\n", err)
					return fmt.Errorf("prep command failed in scene %s", sc.Name)
				}
			}
		}

		// Parse commands from tape file.
		commands, err := tape.ParseCommands(tapePath)
		if err != nil {
			c.Printf("  ✗ Failed to parse tape: %v\n\n", err)
			return fmt.Errorf("failed to parse tape for scene %s: %w", sc.Name, err)
		}

		// Filter to executable commands only.
		executable := tape.FilterExecutable(commands)

		if len(executable) == 0 {
			c.Printf("  (no executable commands found)\n\n")
			continue
		}

		// Execute commands one at a time, stopping on first failure.
		for _, cmd := range executable {
			var result tape.ExecutionResult
			if verbose {
				// Direct execution - output goes straight to terminal, no buffering.
				c.Printf("$ %s\n", cmd.Text)
				result = tape.ExecuteCommandDirect(ctx, cmd, workdir, nil)
				if result.Success {
					c.Printf("\n")
				}
			} else {
				// Buffered execution for non-verbose mode.
				result = tape.ExecuteCommand(ctx, cmd, workdir, nil)
				if result.Success {
					c.Printf("  ✓ %s (%.1fs)\n", cmd.Text, result.Duration.Seconds())
				}
			}

			if !result.Success {
				if !verbose {
					c.Printf("  ✗ %s (%.1fs)\n", cmd.Text, result.Duration.Seconds())
				}
				c.Printf("    Exit code: %d\n", result.ExitCode)
				if !verbose && result.Stderr != "" {
					lines := strings.Split(strings.TrimSpace(result.Stderr), "\n")
					maxLines := 10
					if len(lines) > maxLines {
						lines = lines[:maxLines]
					}
					for _, line := range lines {
						c.Printf("    %s\n", line)
					}
				}
				if result.Error != nil {
					c.Printf("    Error: %v\n", result.Error)
				}
				return fmt.Errorf("command failed in scene %s: %s", sc.Name, cmd.Text)
			}
		}

		c.Printf("\n")
	}

	c.Printf("All commands passed!\n")
	return nil
}
