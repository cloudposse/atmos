package toolchain

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

var pathParser *flags.StandardParser

var pathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print PATH entries for installed tools (alias for 'toolchain env')",
	Long:  `Print PATH entries for all tools configured in .tool-versions. This is an alias for 'toolchain env'.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Bind flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := pathParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Map path flags to env flags:
		// --export → --format bash
		// --json → --format json
		// --relative → --relative
		// (no flags) → --format github (one path per line)
		exportFlag := v.GetBool("export")
		jsonFlag := v.GetBool("json")
		relativeFlag := v.GetBool("relative")

		format := "github" // Default: just paths, one per line.
		if jsonFlag {
			format = "json"
		} else if exportFlag {
			format = "bash"
		}

		return toolchain.EmitEnv(format, relativeFlag, "")
	},
}

func init() {
	// Create parser with path-specific flags.
	pathParser = flags.NewStandardParser(
		flags.WithBoolFlag("export", "", false, "Output in shell export format"),
		flags.WithBoolFlag("json", "", false, "Output in JSON format"),
		flags.WithBoolFlag("relative", "", false, "Use relative paths instead of absolute"),
		flags.WithEnvVars("export", "ATMOS_TOOLCHAIN_EXPORT"),
		flags.WithEnvVars("json", "ATMOS_TOOLCHAIN_JSON"),
		flags.WithEnvVars("relative", "ATMOS_TOOLCHAIN_RELATIVE"),
	)

	// Register flags.
	pathParser.RegisterFlags(pathCmd)

	// Bind flags to Viper.
	if err := pathParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

// PathCommandProvider implements the CommandProvider interface.
type PathCommandProvider struct{}

func (p *PathCommandProvider) GetCommand() *cobra.Command {
	return pathCmd
}

func (p *PathCommandProvider) GetName() string {
	return "path"
}

func (p *PathCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (p *PathCommandProvider) GetFlagsBuilder() flags.Builder {
	return pathParser
}

func (p *PathCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (p *PathCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
