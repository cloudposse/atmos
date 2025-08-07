package toolchain

import (
	"fmt"

	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove <tool>[@<version>]",
	Short: "Remove a tool or a specific version from .tool-versions",
	Long: `Remove a tool or a specific version from the .tool-versions file.

This command removes a tool and all its versions, or a specific version, from the .tool-versions file.

Examples:
  toolchain remove terraform
  toolchain remove hashicorp/terraform
  toolchain remove terraform@1.11.4
  toolchain remove hashicorp/terraform@1.11.4
  toolchain remove --file /path/to/.tool-versions kubectl@1.28.0`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath, _ := cmd.Flags().GetString("file")
		if filePath == "" {
			filePath = GetToolVersionsFilePath()
		}
		input := args[0]
		tool, version, err := ParseToolVersionArg(input)
		if err != nil {
			return err
		}

		toolVersions, err := LoadToolVersions(filePath)
		if err != nil {
			return err
		}

		versions, exists := toolVersions.Tools[tool]
		if !exists {
			return fmt.Errorf("tool '%s' not found in %s", tool, filePath)
		}

		if version == "" {
			// Remove all versions of the tool
			delete(toolVersions.Tools, tool)
			err = SaveToolVersions(filePath, toolVersions)
			if err != nil {
				return err
			}
			cmd.Printf("%s Removed %s from %s\n", checkMark.Render(), tool, filePath)
			return nil
		}

		// Remove only the specified version
		newVersions := make([]string, 0, len(versions))
		removed := false
		for _, v := range versions {
			if v == version {
				removed = true
				continue
			}
			newVersions = append(newVersions, v)
		}
		if !removed {
			return fmt.Errorf("version '%s' not found for tool '%s' in %s", version, tool, filePath)
		}
		if len(newVersions) == 0 {
			delete(toolVersions.Tools, tool)
		} else {
			toolVersions.Tools[tool] = newVersions
		}
		err = SaveToolVersions(filePath, toolVersions)
		if err != nil {
			return err
		}
		cmd.Printf("%s Removed %s@%s from %s\n", checkMark.Render(), tool, version, filePath)
		return nil
	},
}

func init() {
	removeCmd.Flags().String("file", "", "Path to tool-versions file (defaults to global --tool-versions-file)")
}
