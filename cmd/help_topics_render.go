package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

func printLocalFlagsOnly(w io.Writer, cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration, styles *helpStyles) {
	defer perf.Track(atmosConfig, "cmd.printLocalFlagsOnly")()

	localFlags := commandSpecificFlagSet(cmd)
	if localFlags == nil || !localFlags.HasAvailableFlags() {
		return
	}

	fmt.Fprintln(w, styles.heading.Render("FLAGS"))
	fmt.Fprintln(w)
	renderFlags(w, localFlags, styles.commandName, styles.flagName, styles.flagDesc, getTerminalWidth(), atmosConfig)
	fmt.Fprintln(w)
}

func commandSpecificFlagSet(cmd *cobra.Command) *pflag.FlagSet {
	if cmd == nil {
		return nil
	}

	localFlags := cmd.LocalFlags()
	if localFlags == nil {
		return nil
	}

	filtered := pflag.NewFlagSet(cmd.Name(), pflag.ContinueOnError)
	localFlags.VisitAll(func(flag *pflag.Flag) {
		if isRootPersistentFlag(cmd, flag.Name) {
			return
		}
		filtered.AddFlag(flag)
	})
	return filtered
}

func isRootPersistentFlag(cmd *cobra.Command, name string) bool {
	if cmd == nil || cmd.Parent() != nil || name == "" {
		return false
	}
	return cmd.PersistentFlags().Lookup(name) != nil
}

func printHelpForTopic(ctx *helpRenderContext, cmd *cobra.Command, topic helpTopicRequest) {
	if topic.explicit && !topic.valid {
		errUtils.CheckErrorPrintAndExit(
			fmt.Errorf("%w: %q", errUtils.ErrUnknownHelpTopic, topic.raw),
			"Invalid Help Topic",
			"Valid help topics: "+validHelpTopics(),
		)
		return
	}

	switch topic.topic {
	case helpTopicUsage:
		printUsageSection(ctx.writer, cmd, ctx.renderer, ctx.styles)
		printExamples(ctx.writer, cmd, ctx.renderer, ctx.styles)
	case helpTopicFlags:
		printLocalFlagsOnly(ctx.writer, cmd, ctx.atmosConfig, ctx.styles)
		printCompatibilityFlags(ctx.writer, cmd, ctx.styles)
	case helpTopicAll:
		printFullHelp(ctx, cmd)
	default:
		printDefaultHelp(ctx, cmd)
	}
}

func printCommonHelpSections(ctx *helpRenderContext, cmd *cobra.Command) {
	printLogoAndVersion(ctx.writer, ctx.styles)
	printDescription(ctx.writer, cmd, ctx.styles)
	printUsageSection(ctx.writer, cmd, ctx.renderer, ctx.styles)
	printAliases(ctx.writer, cmd, ctx.styles)
	printSubcommandAliases(ctx, cmd)
	printExamples(ctx.writer, cmd, ctx.renderer, ctx.styles)
	printAvailableCommands(ctx, cmd)
	printConfigAliases(ctx, cmd)
}

func printFullHelp(ctx *helpRenderContext, cmd *cobra.Command) {
	printCommonHelpSections(ctx, cmd)
	printFlags(ctx.writer, cmd, ctx.atmosConfig, ctx.styles)
	printCompatibilityFlags(ctx.writer, cmd, ctx.styles)
	printFooter(ctx.writer, cmd, ctx.styles)
}

func printDefaultHelp(ctx *helpRenderContext, cmd *cobra.Command) {
	// settings.terminal.help.filter=false restores the pre-filter full help
	// (FLAGS + GLOBAL FLAGS, no topic hint) — the behavior before PR #2696,
	// journaled in pkg/edition so an edition pin can restore it too.
	if ctx.atmosConfig != nil && !ctx.atmosConfig.Settings.Terminal.Help.Filter {
		printFullHelp(ctx, cmd)
		return
	}
	printCommonHelpSections(ctx, cmd)
	printLocalFlagsOnly(ctx.writer, cmd, ctx.atmosConfig, ctx.styles)
	printCompatibilityFlags(ctx.writer, cmd, ctx.styles)
	printFooter(ctx.writer, cmd, ctx.styles)
	printHelpTopicHint(ctx.writer, ctx.styles)
}

func printHelpTopicHint(w io.Writer, styles *helpStyles) {
	defer perf.Track(nil, "cmd.printHelpTopicHint")()

	usageMsg := "Use `--help=usage` for examples or `--help=all` for all flags and full help."
	usageMsg = renderMarkdownDescription(usageMsg)
	fmt.Fprintf(w, "\n%s\n", styles.muted.Render(usageMsg))
}
