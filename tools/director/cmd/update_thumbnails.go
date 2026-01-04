package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/ffmpeg"
	"github.com/cloudposse/atmos/tools/director/internal/backend"
	vhsCache "github.com/cloudposse/atmos/tools/director/internal/vhs"
)

func updateThumbnailsCmd() *cobra.Command {
	var (
		sceneName string
		dryRun    bool
	)

	cmd := &cobra.Command{
		Use:   "update-thumbnails",
		Short: "Analyze videos and set optimal thumbnails on Cloudflare Stream",
		Long: `Analyzes each video to find the most colorful frame and sets it as the
thumbnail on Cloudflare Stream. This helps ensure thumbnails are visually
appealing and representative of the video content.

The analysis considers:
- Color saturation (prefers colorful frames)
- Scene changes (finds interesting moments)
- Avoids black/white frames at start/end`,
		Example: `
# Update thumbnails for all videos
director update-thumbnails

# Update thumbnail for a specific scene
director update-thumbnails --scene terraform-plan

# Dry run (analyze without updating Stream)
director update-thumbnails --dry-run
`,
		RunE: func(c *cobra.Command, args []string) error {
			ctx := context.Background()

			// Load .env file if it exists.
			_ = godotenv.Load() // Current directory

			// Find demos directory.
			demosDir, err := findDemosDir()
			if err != nil {
				return err
			}

			// Also try demos directory .env.
			envFile := filepath.Join(demosDir, ".env")
			if _, err := os.Stat(envFile); err == nil {
				_ = godotenv.Load(envFile)
			}

			// Ensure ffmpeg is installed.
			if err := ffmpeg.CheckInstalled(); err != nil {
				return err
			}

			// Load cache.
			cacheDir := filepath.Join(demosDir, ".cache")
			cache, err := vhsCache.LoadCache(cacheDir)
			if err != nil {
				return fmt.Errorf("failed to load cache: %w", err)
			}

			// Create Stream backend for API calls.
			var streamBackend *backend.StreamBackend
			if !dryRun {
				streamConfig, err := backend.LoadStreamConfig(nil)
				if err != nil {
					return fmt.Errorf("failed to load Stream config: %w", err)
				}
				streamBackend, err = backend.NewStreamBackend(streamConfig)
				if err != nil {
					return fmt.Errorf("failed to create Stream backend: %w", err)
				}
				// Validate credentials.
				if err := streamBackend.Validate(ctx); err != nil {
					return err
				}
			}

			// Process scenes.
			updated := 0
			for name, scene := range cache.Scenes {
				// Filter by scene name if specified.
				if sceneName != "" && name != sceneName {
					continue
				}

				// Skip scenes without Stream UID.
				if scene.StreamUID == "" {
					continue
				}

				// Find the MP4 output file in cache directory.
				mp4Path := filepath.Join(cacheDir, fmt.Sprintf("%s.mp4", name))

				// Check if file exists.
				if _, err := os.Stat(mp4Path); os.IsNotExist(err) {
					fmt.Printf("⏭️  %s: MP4 file not found at %s\n", name, mp4Path)
					continue
				}

				fmt.Printf("🔍 %s: analyzing video...\n", name)

				// Find the best thumbnail timestamp.
				thumbnailTime, err := ffmpeg.FindBestThumbnailTime(ctx, mp4Path)
				if err != nil {
					fmt.Printf("⚠️  %s: failed to analyze: %v\n", name, err)
					continue
				}

				// Get video duration for percentage calculation.
				duration, err := ffmpeg.GetVideoDuration(ctx, mp4Path)
				if err != nil {
					fmt.Printf("⚠️  %s: failed to get duration: %v\n", name, err)
					continue
				}

				// Calculate percentage for Stream API.
				thumbnailPct := thumbnailTime / duration
				if thumbnailPct < 0 {
					thumbnailPct = 0
				}
				if thumbnailPct > 1 {
					thumbnailPct = 1
				}

				fmt.Printf("   📍 Best frame at %.2fs (%.1f%% of video)\n", thumbnailTime, thumbnailPct*100)

				if dryRun {
					fmt.Printf("   🔹 [dry-run] Would set thumbnail to %.2fs\n", thumbnailTime)
					updated++
					continue
				}

				// Update thumbnail on Stream.
				if err := streamBackend.SetThumbnailTime(ctx, scene.StreamUID, thumbnailPct); err != nil {
					fmt.Printf("⚠️  %s: failed to set thumbnail: %v\n", name, err)
					continue
				}

				// Update cache with thumbnail time.
				scene.ThumbnailTime = thumbnailTime
				cache.Scenes[name] = scene
				updated++

				fmt.Printf("   ✅ Thumbnail updated to %.2fs\n", thumbnailTime)

				// Small delay to avoid rate limiting.
				time.Sleep(100 * time.Millisecond)
			}

			// Save cache.
			if !dryRun && updated > 0 {
				if err := cache.SaveCache(cacheDir); err != nil {
					return fmt.Errorf("failed to save cache: %w", err)
				}
				fmt.Printf("\n✅ Updated thumbnails for %d scenes\n", updated)
			} else if dryRun {
				fmt.Printf("\n🔹 [dry-run] Would update %d scenes\n", updated)
			} else {
				fmt.Printf("\n📋 No scenes needed thumbnail updates\n")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&sceneName, "scene", "s", "", "Update thumbnail for a specific scene only")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Analyze but don't update Stream")

	return cmd
}
