package cmd

import (
	"fmt"

	"github.com/cloudposse/atmos/toolchain"
	"github.com/spf13/cobra"
)

var toolchainRemoveCmd = &cobra.Command{
	Use:   "remove <tool>[@<version>]",
	Short: "Remove a tool or a specific version from .tool-versions",
	Long: `Remove a tool or a specific version from the .tool-versions file.

This command removes a tool and all its versions, or a specific version, from the .tool-versions file.

Examples:
atmos toolchain remove terraform
atmos toolchain remove hashicorp/terraform
atmos toolchain remove terraform@1.11.4
atmos toolchain remove hashicorp/terraform@1.11.4
atmos toolchain remove --file /path/to/.tool-versions kubectl@1.28.0`,
	Args: cobra.ExactArgs(1),
	RunE: runRemoveToolCmd,
}

func init() {
	toolchainRemoveCmd.Flags().String("file", "", "Path to tool-versions file (defaults to global --tool-versions-file)")
}

func runRemoveToolCmd(cmd *cobra.Command, args []string) error {
	filePath, _ := cmd.Flags().GetString("file")
	if filePath != "" {
		atmosConfig.Toolchain.FilePath = filePath
	}

	input := args[0]
	if input == "" {
		return fmt.Errorf("empty tool argument")
	}
	tool, version, err := toolchain.ParseToolVersionArg(input)
	if err != nil {
		return err
	}

	// Call business logic
	err = toolchain.RemoveToolVersion(filePath, tool, version)
	if err != nil {
		return err
	}
	return nil
}
