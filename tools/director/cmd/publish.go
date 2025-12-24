package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/ffmpeg"
	"github.com/cloudposse/atmos/tools/director/internal/backend"
	"github.com/cloudposse/atmos/tools/director/internal/scene"
	vhsCache "github.com/cloudposse/atmos/tools/director/internal/vhs"
)

// runPublish is the shared publish logic that can be called from both the publish command
// and the render command (when --publish flag is used).
// formatFilter limits which formats to publish (nil means all formats).
func runPublish(ctx context.Context, c *cobra.Command, demosDir string, sceneNames []string, force bool, formatFilter []string) error {
	// Load .env file if it exists (check current dir first, then demos dir).
	// Silently ignore if not present in either location.
	_ = godotenv.Load() // Current directory

	// Also try demos directory.
	envFile := filepath.Join(demosDir, ".env")
	if _, err := os.Stat(envFile); err == nil {
		if err := godotenv.Load(envFile); err != nil {
			fmt.Printf("Warning: Failed to load .env file: %v\n", err)
		}
	}

	// Load defaults.yaml to get backend configuration.
	defaultsFile := filepath.Join(demosDir, "defaults.yaml")
	defaultsData, err := os.ReadFile(defaultsFile)
	if err != nil {
		return fmt.Errorf("failed to read defaults.yaml: %w", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(defaultsData, &config); err != nil {
		return fmt.Errorf("failed to parse defaults.yaml: %w", err)
	}

	// Get backend configuration.
	backendConfig, ok := config["backend"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("backend configuration not found in defaults.yaml")
	}

	// Load Stream backend configuration.
	streamConfig, streamErr := backend.LoadStreamConfig(backendConfig)
	var streamBackend *backend.StreamBackend
	if streamErr == nil {
		streamBackend, streamErr = backend.NewStreamBackend(streamConfig)
	}

	// Load R2 backend configuration.
	r2Config, r2Err := backend.LoadR2Config(backendConfig)
	var r2Backend *backend.R2Backend
	if r2Err == nil {
		r2Backend, r2Err = backend.NewR2Backend(r2Config)
	}

	// Require at least one backend to be configured.
	if streamErr != nil && r2Err != nil {
		// Both backends failed - show both errors.
		return fmt.Errorf("no backends configured:\n\nStream: %v\n\nR2: %v", streamErr, r2Err)
	}

	// Show warnings for backends that couldn't be configured (but don't fail).
	if streamErr != nil {
		fmt.Printf("⚠ Stream backend not available: %v\n", streamErr)
		fmt.Println("   → MP4 videos will not be published")
	}
	if r2Err != nil {
		fmt.Printf("⚠ R2 backend not available: %v\n", r2Err)
		fmt.Println("   → GIF/PNG images will not be published")
	}

	// Validate backends.
	if streamBackend != nil {
		fmt.Println("Validating Stream credentials...")
		if err := streamBackend.Validate(ctx); err != nil {
			// Stream validation failed - disable it.
			fmt.Printf("⚠ Stream validation failed: %v\n", err)
			streamBackend = nil
		} else {
			fmt.Println("✓ Stream credentials validated")
		}
	}

	if r2Backend != nil {
		fmt.Println("Validating R2 credentials...")
		if err := r2Backend.Validate(ctx); err != nil {
			// R2 validation failed - disable it.
			fmt.Printf("⚠ R2 validation failed: %v\n", err)
			r2Backend = nil
		} else {
			fmt.Println("✓ R2 credentials validated")
		}
	}

	// Load cache metadata.
	cacheDir := filepath.Join(demosDir, ".cache")
	cache, err := vhsCache.LoadCache(cacheDir)
	if err != nil {
		return fmt.Errorf("failed to load cache: %w", err)
	}

	// Load scenes to get scene names and outputs.
	scenesFile := filepath.Join(demosDir, "scenes.yaml")
	scenesList, err := scene.LoadScenes(scenesFile)
	if err != nil {
		return fmt.Errorf("failed to load scenes: %w", err)
	}

	// Determine which scenes to publish.
	scenesToPublish := make(map[string]*scene.Scene)
	if len(sceneNames) > 0 {
		// Publish specific scenes.
		for _, name := range sceneNames {
			found := false
			for _, sc := range scenesList.Scenes {
				if sc.Name == name {
					scenesToPublish[name] = sc
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("scene not found: %s", name)
			}
		}
	} else {
		// Publish all scenes that have rendered outputs.
		for _, sc := range scenesList.Scenes {
			scenesToPublish[sc.Name] = sc
		}
	}

	fmt.Printf("Publishing VHS demos to Cloudflare backends...\n\n")

	var uploaded, skipped int
	var uploadedToStream, uploadedToR2 int
	var uploadedFiles []string

	// Publish each scene's outputs.
	for sceneName, sc := range scenesToPublish {
		// Get category for hierarchical path structure.
		category := sc.GetCategory()
		if category == "" {
			fmt.Printf("⊘ %s (skipped - no gallery.category configured)\n", sceneName)
			continue
		}

		// Process each output format.
		// Output files are named after the scene name, not the tape filename.
		for _, format := range sc.Outputs {
			// Skip format if not in filter (when filter is set).
			if len(formatFilter) > 0 && !containsFormat(formatFilter, format) {
				continue
			}

			outputFile := filepath.Join(cacheDir, fmt.Sprintf("%s.%s", sceneName, format))

			// Check if file exists.
			if _, err := os.Stat(outputFile); os.IsNotExist(err) {
				continue // Skip if not rendered yet
			}

			// Check if file needs publishing.
			needsPublish, err := cache.NeedsPublish(sceneName, outputFile, force)
			if err != nil {
				return fmt.Errorf("failed to check publish status for %s: %w", outputFile, err)
			}

			if !needsPublish {
				skipped++
				fmt.Printf("⊘ %s.%s (unchanged, skipped)\n", sceneName, format)
				continue
			}

			// Choose backend based on file format.
			var selectedBackend backend.Backend
			var backendName string

			if streamBackend != nil && streamBackend.SupportsFormat(format) {
				selectedBackend = streamBackend
				backendName = "Stream"
			} else if r2Backend != nil && r2Backend.SupportsFormat(format) {
				selectedBackend = r2Backend
				backendName = "R2"
			} else {
				// No backend supports this format - provide helpful message.
				if format == "gif" || format == "png" || format == "svg" {
					fmt.Printf("⊘ %s.%s (skipped - requires R2 backend, see .env)\n", sceneName, format)
				} else {
					fmt.Printf("⊘ %s.%s (no backend supports this format)\n", sceneName, format)
				}
				continue
			}

			// Build remote path with hierarchical structure: {category}/{scene-name}.{format}
			// For R2: prefix (demos/) will be added by backend's buildKey()
			// For Stream: Used as metadata name
			remotePath := fmt.Sprintf("%s/%s.%s", category, sceneName, format)

			// Build human-readable title from scene description or name.
			// This is displayed in Cloudflare Stream dashboard.
			title := sc.Description
			if title == "" {
				// Fall back to scene name if no description.
				title = sceneName
			}

			// Get public URL (preliminary, will be updated after upload for Stream).
			publicURL := selectedBackend.GetPublicURL(remotePath)

			// Upload to selected backend.
			if err := selectedBackend.Upload(ctx, outputFile, remotePath, title); err != nil {
				return fmt.Errorf("failed to upload %s to %s: %w", outputFile, backendName, err)
			}

			// Get Stream metadata if uploaded to Stream.
			var streamMetadata *vhsCache.StreamMetadata
			if backendName == "Stream" {
				if sb, ok := selectedBackend.(*backend.StreamBackend); ok {
					if meta := sb.GetLastMetadata(); meta != nil {
						// Update public URL to use actual UID.
						publicURL = meta.Preview

						// Calculate video duration from local file.
						var duration float64
						if dur, err := ffmpeg.GetVideoDuration(ctx, outputFile); err == nil {
							duration = dur
						}

						// Convert to cache metadata format.
						streamMetadata = &vhsCache.StreamMetadata{
							UID:               meta.UID,
							CustomerSubdomain: meta.CustomerSubdomain,
							Duration:          duration,
						}
					}
				}
			}

			// Update cache.
			if err := cache.UpdatePublish(sceneName, outputFile, publicURL, streamMetadata); err != nil {
				return fmt.Errorf("failed to update cache for %s: %w", outputFile, err)
			}

			uploaded++
			if backendName == "Stream" {
				uploadedToStream++
			} else {
				uploadedToR2++
			}
			uploadedFiles = append(uploadedFiles, publicURL)
			fmt.Printf("✓ %s.%s → %s (%s)\n", sceneName, format, publicURL, backendName)
		}

		// Upload MP3 audio file if scene has audio config and R2 is available.
		if sc.Audio != nil && sc.Audio.Source != "" && r2Backend != nil {
			audioFile := filepath.Join(demosDir, sc.Audio.Source)

			// Check if audio file exists.
			if _, err := os.Stat(audioFile); err == nil {
				// Check if MP3 needs publishing.
				needsPublish, err := cache.NeedsPublish(sceneName, audioFile, force)
				if err != nil {
					return fmt.Errorf("failed to check publish status for %s: %w", audioFile, err)
				}

				if needsPublish {
					// Build remote path: {category}/{scene-name}.mp3
					remotePath := fmt.Sprintf("%s/%s.mp3", category, sceneName)
					title := fmt.Sprintf("%s audio", sceneName)

					publicURL := r2Backend.GetPublicURL(remotePath)

					if err := r2Backend.Upload(ctx, audioFile, remotePath, title); err != nil {
						return fmt.Errorf("failed to upload audio %s to R2: %w", audioFile, err)
					}

					// Update cache for audio file.
					if err := cache.UpdatePublish(sceneName, audioFile, publicURL, nil); err != nil {
						return fmt.Errorf("failed to update cache for audio %s: %w", audioFile, err)
					}

					uploaded++
					uploadedToR2++
					uploadedFiles = append(uploadedFiles, publicURL)
					fmt.Printf("✓ %s.mp3 → %s (R2)\n", sceneName, publicURL)
				} else {
					skipped++
					fmt.Printf("⊘ %s.mp3 (unchanged, skipped)\n", sceneName)
				}
			}
		}
	}

	// Save cache metadata.
	if err := cache.SaveCache(cacheDir); err != nil {
		return fmt.Errorf("failed to save cache: %w", err)
	}

	// Print summary.
	fmt.Printf("\n")
	summary := fmt.Sprintf("Published %d file", uploaded)
	if uploaded != 1 {
		summary += "s"
	}
	if uploadedToStream > 0 && uploadedToR2 > 0 {
		summary += fmt.Sprintf(" (%d to Stream, %d to R2)", uploadedToStream, uploadedToR2)
	} else if uploadedToStream > 0 {
		summary += " to Stream"
	} else if uploadedToR2 > 0 {
		summary += " to R2"
	}
	if skipped > 0 {
		summary += fmt.Sprintf(", %d skipped", skipped)
	}
	fmt.Println(summary)

	// Print URLs if any were uploaded.
	if len(uploadedFiles) > 0 {
		fmt.Printf("\nPublic URLs:\n")
		for _, url := range uploadedFiles {
			fmt.Printf("  %s\n", url)
		}
	}

	return nil
}

func publishCmd() *cobra.Command {
	var (
		force       bool
		dryRun      bool
		backendType string
	)

	cmd := &cobra.Command{
		Use:   "publish [scene-names...]",
		Short: "Publish rendered demos to Cloudflare Stream or R2",
		Long: `Publish rendered VHS demos to Cloudflare Stream (for videos) or R2 (for images).

By default, publishes all rendered scenes from the .cache directory.
Specify scene names to publish only those scenes.

Uses smart caching - only uploads files that have changed since last publish
(unless --force is specified).

Automatically routes files to the appropriate backend:
  - MP4 videos → Cloudflare Stream (adaptive streaming, transcoding)
  - GIF/PNG images → Cloudflare R2 (object storage)

Required environment variables:
  For Stream (videos):
    - CLOUDFLARE_ACCOUNT_ID
    - CLOUDFLARE_STREAM_API_TOKEN

  For R2 (images):
    - CLOUDFLARE_ACCOUNT_ID
    - CLOUDFLARE_R2_ACCESS_KEY_ID
    - CLOUDFLARE_R2_SECRET_ACCESS_KEY`,
		Example: `
# Publish all rendered demos
director publish

# Publish specific scenes
director publish terraform-plan describe-stacks

# Force re-upload all files
director publish --force

# Dry run (show what would be uploaded)
director publish --dry-run
`,
		RunE: func(c *cobra.Command, args []string) error {
			ctx := context.Background()

			demosDir, err := findDemosDir()
			if err != nil {
				return err
			}

			if dryRun {
				return runPublishDryRun(ctx, c, demosDir, args, force)
			}

			return runPublish(ctx, c, demosDir, args, force, nil)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Re-upload all files (ignore cache)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be uploaded without uploading")
	cmd.Flags().StringVar(&backendType, "backend", "", "Backend type override (not implemented)")

	return cmd
}

// runPublishDryRun shows what would be uploaded without actually uploading.
func runPublishDryRun(ctx context.Context, c *cobra.Command, demosDir string, sceneNames []string, force bool) error {
	// Load .env file if it exists.
	_ = godotenv.Load()
	envFile := filepath.Join(demosDir, ".env")
	if _, err := os.Stat(envFile); err == nil {
		_ = godotenv.Load(envFile)
	}

	// Load defaults.yaml to get backend configuration.
	defaultsFile := filepath.Join(demosDir, "defaults.yaml")
	defaultsData, err := os.ReadFile(defaultsFile)
	if err != nil {
		return fmt.Errorf("failed to read defaults.yaml: %w", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(defaultsData, &config); err != nil {
		return fmt.Errorf("failed to parse defaults.yaml: %w", err)
	}

	backendConfig, ok := config["backend"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("backend configuration not found in defaults.yaml")
	}

	// Load backends.
	streamConfig, streamErr := backend.LoadStreamConfig(backendConfig)
	var streamBackend *backend.StreamBackend
	if streamErr == nil {
		streamBackend, _ = backend.NewStreamBackend(streamConfig)
	}

	r2Config, r2Err := backend.LoadR2Config(backendConfig)
	var r2Backend *backend.R2Backend
	if r2Err == nil {
		r2Backend, _ = backend.NewR2Backend(r2Config)
	}

	if streamErr != nil && r2Err != nil {
		return fmt.Errorf("no backends configured:\n\nStream: %v\n\nR2: %v", streamErr, r2Err)
	}

	// Load cache and scenes.
	cacheDir := filepath.Join(demosDir, ".cache")
	cache, err := vhsCache.LoadCache(cacheDir)
	if err != nil {
		return fmt.Errorf("failed to load cache: %w", err)
	}

	scenesFile := filepath.Join(demosDir, "scenes.yaml")
	scenesList, err := scene.LoadScenes(scenesFile)
	if err != nil {
		return fmt.Errorf("failed to load scenes: %w", err)
	}

	scenesToPublish := make(map[string]*scene.Scene)
	if len(sceneNames) > 0 {
		for _, name := range sceneNames {
			for _, sc := range scenesList.Scenes {
				if sc.Name == name {
					scenesToPublish[name] = sc
					break
				}
			}
		}
	} else {
		for _, sc := range scenesList.Scenes {
			scenesToPublish[sc.Name] = sc
		}
	}

	fmt.Printf("Dry run - showing what would be published...\n\n")

	var wouldUpload int
	for sceneName, sc := range scenesToPublish {
		// Get category for hierarchical path structure.
		category := sc.GetCategory()
		if category == "" {
			fmt.Printf("⊘ %s (would skip - no gallery.category configured)\n", sceneName)
			continue
		}

		for _, format := range sc.Outputs {
			outputFile := filepath.Join(cacheDir, fmt.Sprintf("%s.%s", sceneName, format))

			if _, err := os.Stat(outputFile); os.IsNotExist(err) {
				continue
			}

			needsPublish, _ := cache.NeedsPublish(sceneName, outputFile, force)
			if !needsPublish {
				fmt.Printf("⊘ %s.%s (unchanged, would skip)\n", sceneName, format)
				continue
			}

			var selectedBackend backend.Backend
			var backendName string

			if streamBackend != nil && streamBackend.SupportsFormat(format) {
				selectedBackend = streamBackend
				backendName = "Stream"
			} else if r2Backend != nil && r2Backend.SupportsFormat(format) {
				selectedBackend = r2Backend
				backendName = "R2"
			} else {
				fmt.Printf("⊘ %s.%s (no backend supports this format)\n", sceneName, format)
				continue
			}

			remotePath := fmt.Sprintf("%s/%s.%s", category, sceneName, format)
			publicURL := selectedBackend.GetPublicURL(remotePath)
			fmt.Printf("⊙ %s.%s → %s (%s, dry run)\n", sceneName, format, publicURL, backendName)
			wouldUpload++
		}

		// Check MP3 audio file for dry run.
		if sc.Audio != nil && sc.Audio.Source != "" && r2Backend != nil {
			audioFile := filepath.Join(demosDir, sc.Audio.Source)

			if _, err := os.Stat(audioFile); err == nil {
				needsPublish, _ := cache.NeedsPublish(sceneName, audioFile, force)
				if needsPublish {
					remotePath := fmt.Sprintf("%s/%s.mp3", category, sceneName)
					publicURL := r2Backend.GetPublicURL(remotePath)
					fmt.Printf("⊙ %s.mp3 → %s (R2, dry run)\n", sceneName, publicURL)
					wouldUpload++
				} else {
					fmt.Printf("⊘ %s.mp3 (unchanged, would skip)\n", sceneName)
				}
			}
		}
	}

	fmt.Printf("\nDry run complete: would upload %d files\n", wouldUpload)
	return nil
}

// containsFormat checks if the filter contains the given format.
func containsFormat(filter []string, format string) bool {
	for _, f := range filter {
		if f == format {
			return true
		}
	}
	return false
}
