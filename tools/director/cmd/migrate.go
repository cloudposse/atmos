package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	vhsCache "github.com/cloudposse/atmos/tools/director/internal/vhs"
)

func migrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate cache metadata to latest schema",
		Long: `Migrate cache metadata to the latest schema version.

Currently supports:
  - Stream UID migration: Converts legacy stream_uid fields to
    stream_versions array for UID history tracking.

This is safe to run multiple times - already migrated data is skipped.`,
		Example: `
# Migrate cache metadata
director migrate
`,
		RunE: func(c *cobra.Command, args []string) error {
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

			// Migrate Stream UIDs.
			migrated := cache.MigrateStreamUIDs()
			if migrated > 0 {
				if err := cache.SaveCache(cacheDir); err != nil {
					return fmt.Errorf("failed to save cache: %w", err)
				}
				fmt.Printf("✓ Migrated %d scene(s) to stream_versions format\n", migrated)
			} else {
				fmt.Println("✓ No scenes to migrate (already migrated or no Stream UIDs)")
			}

			return nil
		},
	}

	return cmd
}
