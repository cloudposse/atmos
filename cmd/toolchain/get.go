package toolchain

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/toolchain"
)

var (
	showAllVersions bool
	versionLimit    int
)

var getCmd = &cobra.Command{
	Use:   "get [tool]",
	Short: "Get version information for a tool",
	Long:  `Display version information for a tool from .tool-versions file.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		toolName := ""
		if len(args) > 0 {
			toolName = args[0]
		}
		// Read from viper to respect config precedence (file/env/flag).
		all := viper.GetBool("toolchain.get.all")
		limit := viper.GetInt("toolchain.get.limit")
		return toolchain.ListToolVersions(all, limit, toolName)
	},
}

func init() {
	getCmd.Flags().BoolVar(&showAllVersions, "all", false, "Show all available versions")
	getCmd.Flags().IntVar(&versionLimit, "limit", 10, "Limit number of versions to display")

	// Bind flags to viper for config precedence.
	if err := viper.BindPFlag("toolchain.get.all", getCmd.Flags().Lookup("all")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding toolchain.get.all flag: %v\n", err)
	}
	if err := viper.BindPFlag("toolchain.get.limit", getCmd.Flags().Lookup("limit")); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding toolchain.get.limit flag: %v\n", err)
	}

	// Bind environment variables.
	if err := viper.BindEnv("toolchain.get.all", "ATMOS_TOOLCHAIN_GET_ALL"); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding ATMOS_TOOLCHAIN_GET_ALL environment variable: %v\n", err)
	}
	if err := viper.BindEnv("toolchain.get.limit", "ATMOS_TOOLCHAIN_GET_LIMIT"); err != nil {
		fmt.Fprintf(os.Stderr, "Error binding ATMOS_TOOLCHAIN_GET_LIMIT environment variable: %v\n", err)
	}

	// Set defaults via viper.
	viper.SetDefault("toolchain.get.all", false)
	viper.SetDefault("toolchain.get.limit", 10)
}
