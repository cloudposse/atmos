package toolchain

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

const (
	defaultVersionLimit = 10
)

var (
	getParser           *flags.StandardParser
	supportedGetFormats = []string{"table", "plain", "json"}
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
		format := v.GetString("format")

		if !slices.Contains(supportedGetFormats, format) {
			return fmt.Errorf("%w: %q (supported: %v)", errUtils.ErrInvalidFlagValue, format, supportedGetFormats)
		}
		if format == "plain" && all {
			return errUtils.ErrToolchainPlainFormatWithAllFlag
		}

		return toolchain.ListToolVersions(all, limit, toolName, format)
	},
}

func init() {
	// Create parser with get-specific flags.
	getParser = flags.NewStandardParser(
		flags.WithBoolFlag("all", "", false, "Show all available versions"),
		flags.WithIntFlag("limit", "", defaultVersionLimit, "Limit number of versions to display"),
		flags.WithStringFlag("format", "f", "table", fmt.Sprintf("Output format: %v", supportedGetFormats)),
		flags.WithEnvVars("all", "ATMOS_TOOLCHAIN_ALL"),
		flags.WithEnvVars("limit", "ATMOS_TOOLCHAIN_LIMIT"),
		flags.WithEnvVars("format", "ATMOS_TOOLCHAIN_GET_FORMAT"),
	)

	// Register flags.
	getParser.RegisterFlags(getCmd)

	// Bind flags to Viper.
	if err := getParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register shell completion for the format flag.
	if err := getCmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return supportedGetFormats, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		panic(err)
	}
}

// GetCommandProvider implements the CommandProvider interface.
type GetCommandProvider struct{}

func (g *GetCommandProvider) GetCommand() *cobra.Command {
	return getCmd
}

func (g *GetCommandProvider) GetName() string {
	return "get"
}

func (g *GetCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (g *GetCommandProvider) GetFlagsBuilder() flags.Builder {
	return getParser
}

func (g *GetCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (g *GetCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
