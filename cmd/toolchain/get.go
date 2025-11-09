package toolchain

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/toolchain"
)

var (
	getParser *flags.StandardParser
)

var getCmd = &cobra.Command{
	Use:   "get [tool]",
	Short: "Get version information for a tool",
	Long:  `Display version information for a tool from .tool-versions file.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Bind flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := getParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		toolName := ""
		if len(args) > 0 {
			toolName = args[0]
		}

		// Read from viper to respect config precedence (file/env/flag).
		all := v.GetBool("all")
		limit := v.GetInt("limit")

		return toolchain.ListToolVersions(all, limit, toolName)
	},
}

func init() {
	// Create parser with get-specific flags.
	getParser = flags.NewStandardParser(
		flags.WithBoolFlag("all", "", false, "Show all available versions"),
		flags.WithIntFlag("limit", "", 10, "Limit number of versions to display"),
		flags.WithEnvVars("all", "ATMOS_TOOLCHAIN_ALL"),
		flags.WithEnvVars("limit", "ATMOS_TOOLCHAIN_LIMIT"),
	)

	// Register flags.
	getParser.RegisterFlags(getCmd)

	// Bind flags to Viper.
	if err := getParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
