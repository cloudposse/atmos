package toolchain

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var whichCmd = &cobra.Command{
	Use:   "which <tool>",
	Short: "Display the path to an executable",
	Long: `Display the path to an executable for a given tool.

This command shows the full path to the binary for a tool that is configured
in .tool-versions and installed via toolchain.

Examples:
  toolchain which terraform
  toolchain which hashicorp/terraform
  toolchain which kubectl`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		toolName := args[0]

		// Check if the tool is configured in .tool-versions
		toolVersions, err := LoadToolVersions(GetToolVersionsFilePath())
		if err != nil {
			return fmt.Errorf("failed to load .tool-versions file: %w", err)
		}

		versions, exists := toolVersions.Tools[toolName]
		if !exists || len(versions) == 0 {
			return fmt.Errorf("tool '%s' not configured in .tool-versions", toolName)
		}

		// Use the most recent version
		version := versions[len(versions)-1]

		// Now that we know the tool is configured, use the installer to resolve the canonical name
		// and get the binary path
		installer := NewInstaller()
		owner, repo, err := installer.parseToolSpec(toolName)
		if err != nil {
			return fmt.Errorf("failed to resolve tool '%s': %w", toolName, err)
		}

		binaryPath := installer.getBinaryPath(owner, repo, version)

		// Check if the binary exists
		if _, err := os.Stat(binaryPath); err != nil {
			return fmt.Errorf("tool '%s' is configured but not installed", toolName)
		}

		cmd.Println(binaryPath)
		return nil
	},
}

func init() {
	// No flags needed for which command
}
