package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add <tool> <version>",
	Short: "Add or update a tool and version in .tool-versions",
	Long: `Add or update a tool and version in the .tool-versions file.

This command adds a tool and its version to the .tool-versions file. If the tool
already exists, it will be updated with the new version.

The tool will be validated against the registry to ensure it exists before being added.

Examples:
  toolchain add terraform 1.9.8
  toolchain add hashicorp/terraform 1.11.4
  toolchain add --file /path/to/.tool-versions kubectl 1.28.0`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath, _ := cmd.Flags().GetString("file")
		if filePath == "" {
			filePath = GetToolVersionsFilePath()
		}
		tool := args[0]
		version := args[1]

		// Validate that the tool exists in the registry
		installer := NewInstaller()
		owner, repo, err := installer.parseToolSpec(tool)
		if err != nil {
			return fmt.Errorf("failed to resolve tool '%s': %w", tool, err)
		}

		// Check if the tool exists in the registry
		_, err = installer.findTool(owner, repo, version)
		if err != nil {
			return fmt.Errorf("tool '%s' not found in registry: %w", tool, err)
		}

		// Add the tool to .tool-versions
		err = AddToolToVersions(filePath, tool, version)
		if err != nil {
			return err
		}
		cmd.Printf("%s Added/updated %s %s in %s\n", checkMark.Render(), tool, version, filePath)
		return nil
	},
}

func init() {
	addCmd.Flags().String("file", "", "Path to tool-versions file (defaults to global --tool-versions-file)")
}
