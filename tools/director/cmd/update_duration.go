package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/ffmpeg"
	vhsCache "github.com/cloudposse/atmos/tools/director/internal/vhs"
)

func updateDurationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-duration",
		Short: "Update video durations in cache metadata",
		Long: `Update video durations for all cached MP4 files.

This command scans all cached MP4 files and updates their duration
in the cache metadata. Use this after adding duration tracking to
existing videos that were published before duration was recorded.

Requires ffprobe to be installed.`,
		Example: `
# Update durations for all cached videos
director update-duration
`,
		RunE: func(c *cobra.Command, args []string) error {
			ctx := context.Background()

			demosDir, err := findDemosDir()
			if err != nil {
				return err
			}

			cacheDir := filepath.Join(demosDir, ".cache")

			// Load cache metadata.
			cache, err := vhsCache.LoadCache(cacheDir)
			if err != nil {
				return fmt.Errorf("failed to load cache: %w", err)
			}

			updated := 0
			for sceneName, sceneHash := range cache.Scenes {
				// Only process scenes with Stream metadata (MP4 videos).
				if sceneHash.StreamUID == "" {
					continue
				}

				// Skip if duration already set.
				if sceneHash.Duration > 0 {
					fmt.Printf("⊘ %s (duration already set: %.1fs)\n", sceneName, sceneHash.Duration)
					continue
				}

				// Find the MP4 file in cache.
				mp4Path := filepath.Join(cacheDir, fmt.Sprintf("%s.mp4", sceneName))

				// Calculate duration.
				duration, err := ffmpeg.GetVideoDuration(ctx, mp4Path)
				if err != nil {
					fmt.Printf("⚠ %s: failed to get duration: %v\n", sceneName, err)
					continue
				}

				// Update cache.
				sceneHash.Duration = duration
				cache.Scenes[sceneName] = sceneHash
				updated++
				fmt.Printf("✓ %s: %.1fs\n", sceneName, duration)
			}

			// Save cache metadata.
			if updated > 0 {
				if err := cache.SaveCache(cacheDir); err != nil {
					return fmt.Errorf("failed to save cache: %w", err)
				}
				fmt.Printf("\n✓ Updated duration for %d scene(s)\n", updated)
			} else {
				fmt.Println("\n✓ No scenes needed duration updates")
			}

			return nil
		},
	}

	return cmd
}
