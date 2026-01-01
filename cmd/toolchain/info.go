package toolchain

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/toolchain"
)

var infoParser *flags.StandardParser

var infoCmd = &cobra.Command{
	Use:   "info <tool>",
	Short: "Show information about a tool",
	Long:  `Display detailed information about a tool from the registry.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Bind flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := infoParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		outputFormat := v.GetString("output")
		return toolchain.InfoExec(args[0], outputFormat)
	},
}

func init() {
	// Create parser with info-specific flags.
	infoParser = flags.NewStandardParser(
		flags.WithStringFlag("output", "o", "table", "Output format (table, yaml, json)"),
		flags.WithEnvVars("output", "ATMOS_TOOLCHAIN_OUTPUT"),
	)

	// Register flags.
	infoParser.RegisterFlags(infoCmd)

	// Bind flags to Viper.
	if err := infoParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

// InfoCommandProvider implements the CommandProvider interface.
type InfoCommandProvider struct{}

func (i *InfoCommandProvider) GetCommand() *cobra.Command {
	return infoCmd
}

func (i *InfoCommandProvider) GetName() string {
	return "info"
}

func (i *InfoCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (i *InfoCommandProvider) GetFlagsBuilder() flags.Builder {
	return infoParser
}

func (i *InfoCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (i *InfoCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
