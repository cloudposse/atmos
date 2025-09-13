package cmd

import (
	"github.com/cloudposse/atmos/toolchain"
	"github.com/spf13/cobra"
)

package cmd

import (
    "github.com/cloudposse/atmos/toolchain"
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

const defaultVersionsLimit = 50

var toolchainVersionsCmd = &cobra.Command{
    Use:   "versions <tool>",
    Short: "List available or configured versions for a tool",
    Args:  cobra.ExactArgs(1), // Requires exactly one argument (tool name)
    RunE: func(cmd *cobra.Command, args []string) error {
        filePath, _ := cmd.Flags().GetString("tool-versions")
        showAll, _ := cmd.Flags().GetBool("all")
        limit, _ := cmd.Flags().GetInt("versions-limit")
        toolName := args[0]
        if filePath != "" {
            atmosConfig.Toolchain.FilePath = filePath
        }
        toolchain.SetAtmosConfig(&atmosConfig)
        return toolchain.ListToolVersions(showAll, limit, toolName)
    },
}

func init() {
    toolchainVersionsCmd.Flags().String("tool-versions", "", "Path to tool-versions file (overrides --tool-versions).")
    toolchainVersionsCmd.Flags().Bool("all", false, "Fetch all available versions from GitHub API.")
    toolchainVersionsCmd.Flags().Int("versions-limit", defaultVersionsLimit, "Maximum number of versions to fetch when using --all.")

    _ = viper.BindPFlag("toolchain.file_path", toolchainVersionsCmd.Flags().Lookup("tool-versions"))
    _ = viper.BindEnv("toolchain.file_path", "ATMOS_TOOLCHAIN_FILE_PATH")
    _ = viper.BindPFlag("toolchain.versions_limit", toolchainVersionsCmd.Flags().Lookup("versions-limit"))
    _ = viper.BindEnv("toolchain.versions_limit", "ATMOS_TOOLCHAIN_VERSIONS_LIMIT")
}

func init() {
	toolchainGetCmd.Flags().String("file", "", "Path to tool-versions file (defaults to global --tool-versions-file)")
	toolchainGetCmd.Flags().Bool("all", false, "Fetch all available versions from GitHub API")
	toolchainGetCmd.Flags().Int("limit", 50, "Maximum number of versions to fetch when using --all")
}
