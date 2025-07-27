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
		count := 0
		err := filepath.Walk(toolsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if path != toolsDir {
				count++
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
		cmd.Printf("%s Deleted %d files/directories from %s\n", checkMark.Render(), count, toolsDir)
		return nil
	},
}
