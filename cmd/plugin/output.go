package plugin

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	pluginoutput "github.com/cloudposse/atmos/pkg/plugin/output"
)

const outputFormatFlag = "format"

func newOutputParser(options ...flags.Option) *flags.StandardParser {
	options = append(
		options,
		flags.WithStringFlag(outputFormatFlag, "", "table", "Output format: table, json, yaml, csv, tsv"),
		flags.WithEnvVars(outputFormatFlag, "ATMOS_PLUGIN_FORMAT"),
	)
	return flags.NewStandardParser(options...)
}

func registerOutputFlags(command *cobra.Command, parser *flags.StandardParser) {
	parser.RegisterFlags(command)
	if err := command.RegisterFlagCompletionFunc(outputFormatFlag, func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"table", "json", "yaml", "csv", "tsv"}, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		panic(err)
	}
}

func parseOutputFlags(command *cobra.Command, parser *flags.StandardParser) (*viper.Viper, error) {
	values := viper.New()
	if err := parser.BindFlagsToViper(command, values); err != nil {
		return nil, err
	}
	if err := pluginoutput.ValidateFormat(values.GetString(outputFormatFlag)); err != nil {
		return nil, err
	}
	return values, nil
}
