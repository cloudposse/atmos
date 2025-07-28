package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove all installed tools by deleting the .tools directory",
	Long: `Remove all installed tools by deleting the .tools directory.

This command will:
- Count the number of files/directories in the .tools directory
- Delete the entire .tools directory and all its contents
- Display a summary of what was deleted

Use this command to completely clean up all installed tools.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		toolsDir := GetToolsDirPath()
		homeDir, _ := os.UserHomeDir()
		cacheDir := filepath.Join(homeDir, ".cache", "tools-cache")
		tempCacheDir := filepath.Join(os.TempDir(), "tools-cache")

		// Clean tools directory
		toolsCount := 0
		err := filepath.Walk(toolsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if path != toolsDir {
				toolsCount++
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to count files in %s: %w", toolsDir, err)
		}
		err = os.RemoveAll(toolsDir)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete %s: %w", toolsDir, err)
		}

		// Clean cache directories
		cacheCount := 0
		err = filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if path != cacheDir {
				cacheCount++
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			// Log but don't fail
			cmd.Printf("Warning: failed to count files in %s: %v\n", cacheDir, err)
		}
		err = os.RemoveAll(cacheDir)
		if err != nil && !os.IsNotExist(err) {
			cmd.Printf("Warning: failed to delete %s: %v\n", cacheDir, err)
		}

		tempCacheCount := 0
		err = filepath.Walk(tempCacheDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if path != tempCacheDir {
				tempCacheCount++
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			// Log but don't fail
			cmd.Printf("Warning: failed to count files in %s: %v\n", tempCacheDir, err)
		}
		err = os.RemoveAll(tempCacheDir)
		if err != nil && !os.IsNotExist(err) {
			cmd.Printf("Warning: failed to delete %s: %v\n", tempCacheDir, err)
		}

		cmd.Printf("%s Deleted %d files/directories from %s\n", checkMark.Render(), toolsCount, toolsDir)
		if cacheCount > 0 {
			cmd.Printf("%s Deleted %d files from %s cache\n", checkMark.Render(), cacheCount, cacheDir)
		}
		if tempCacheCount > 0 {
			cmd.Printf("%s Deleted %d files from %s cache\n", checkMark.Render(), tempCacheCount, tempCacheDir)
		}
		return nil
	},
}
