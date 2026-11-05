package toolchain

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

const aliasesFormatFlag = "format"

var (
	aliasesParser *flags.StandardParser
	aliasesCmd    = &cobra.Command{
		Use:   "aliases",
		Short: "List built-in and configured tool aliases",
		Long: `List effective tool aliases from the merged Atmos configuration and built-in defaults.
Configured aliases override built-in aliases with the same name. Targets are shown as
configured, without resolving them through registries or installing tools.`,
		Example: "atmos toolchain aliases\natmos toolchain aliases --format json\natmos toolchain aliases --format yaml",
		Args:    cobra.NoArgs,
		RunE:    executeAliases,
	}
)

func init() {
	aliasesParser = flags.NewStandardParser(
		flags.WithStringFlag(aliasesFormatFlag, "f", "table", "Output format (table, json, yaml)"),
		flags.WithEnvVars(aliasesFormatFlag, "ATMOS_TOOLCHAIN_ALIASES_FORMAT"),
	)
	aliasesParser.RegisterFlags(aliasesCmd)
	if err := aliasesParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
	if err := aliasesCmd.RegisterFlagCompletionFunc(aliasesFormatFlag, func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"table", "json", "yaml"}, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		panic(err)
	}
	internal.RegisterCommandMetadata(aliasesCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/toolchain/aliases"))
}

func executeAliases(cmd *cobra.Command, _ []string) error {
	defer perf.Track(nil, "toolchain.executeAliases")()

	v := viper.GetViper()
	if err := aliasesParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}
	return writeAliases(effectiveAliases(toolchain.GetAtmosConfig()), strings.ToLower(v.GetString(aliasesFormatFlag)))
}

func effectiveAliases(config *schema.AtmosConfiguration) map[string]string {
	aliases := maps.Clone(toolchain.BuiltinAliases)
	if config != nil {
		maps.Copy(aliases, config.Toolchain.Aliases)
	}
	return aliases
}

func writeAliases(aliases map[string]string, format string) error {
	switch format {
	case "json":
		return data.WriteJSON(aliases)
	case "yaml":
		return data.WriteYAML(aliases)
	case "table":
		return data.Write(formatAliasesTable(aliases))
	default:
		return fmt.Errorf("%w: %q (supported: table, json, yaml)", errUtils.ErrInvalidFlagValue, format)
	}
}

func formatAliasesTable(aliases map[string]string) string {
	const heading = "ALIAS"
	width := len(heading)
	for alias := range aliases {
		width = max(width, len(alias))
	}
	var output strings.Builder
	fmt.Fprintf(&output, "%-*s  TARGET\n", width, heading)
	for _, alias := range slices.Sorted(maps.Keys(aliases)) {
		fmt.Fprintf(&output, "%-*s  %s\n", width, alias, aliases[alias])
	}
	return output.String()
}
