package cmd

import (
	"github.com/cloudposse/atmos/toolchain"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var toolchainAddCmd = &cobra.Command{
	Use:   "add <tool> <version>",
	Short: "Add or update a tool and version in .tool-versions",
	Long: `Add or update a tool and version in the .tool-versions file.

This command adds a tool and its version to the .tool-versions file. If the tool
already exists, it will be updated with the new version.

The tool will be validated against the registry to ensure it exists before being added.
`,
	Args: cobra.ExactArgs(2),
	RunE: runAddToolCmd,
}

func runAddToolCmd(cmd *cobra.Command, args []string) error {
	filePath, _ := cmd.Flags().GetString("file")
	if filePath != "" {
		atmosConfig.Toolchain.FilePath = filePath
	}
	tool := args[0]
	version := args[1]
	// Call the business logic
	if err := toolchain.AddToolVersion(tool, version); err != nil {
		return err
	}
	return nil
}

func init() {
	toolchainAddCmd.Flags().String("file", "", "Path to tool-versions file (defaults to global --tool-versions-file)")
	_ = viper.BindEnv("toolchain.file_path", "TOOLCHAIN_PATH_RELATIVE", "ATMOS_TOOLCHAIN_PATH_RELATIVE")
}
