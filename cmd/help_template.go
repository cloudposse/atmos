package cmd

import (
	"fmt"
	"io"
	"runtime"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	atmosansi "github.com/cloudposse/atmos/pkg/ansi"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	markdown "github.com/cloudposse/atmos/pkg/ui/markdown"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/version"
)

// Help formatting layout constants.
const (
	commandListLeftPad        = 6   // Left padding for command list entries.
	commandDescriptionSpacing = 2   // Spacing between command name and description.
	minDescriptionWidth       = 40  // Minimum width for description text.
	spaceChar                 = " " // Space character for padding.
)

// Environment variable name/value constants used by help rendering tests to force a deterministic, colorless output.
const (
	envNoColor = "NO_COLOR"
	valueOne   = "1"
)

// Command annotation constants.
const (
	annotationExperimental  = "experimental"
	annotationConfigAlias   = "configAlias"
	annotationCustomCommand = "customCommand"
	annotationValueTrue     = "true"
)

// isExperimentalCommand checks if a command has the experimental annotation.
func isExperimentalCommand(cmd *cobra.Command) bool {
	return cmd.Annotations != nil && cmd.Annotations[annotationExperimental] == annotationValueTrue
}

// helpStyles holds the styled text renderers for help output.
// Uses theme-aware styles from theme.GetCurrentStyles().
type helpStyles struct {
	heading     lipgloss.Style // Section headings (USAGE, FLAGS, etc.)
	commandName lipgloss.Style // Command names in lists
	commandDesc lipgloss.Style // Command descriptions
	flagName    lipgloss.Style // Flag names
	flagDesc    lipgloss.Style // Flag descriptions
	muted       lipgloss.Style // Muted text (footer messages)
}

// helpRenderContext holds the rendering context for help output.
type helpRenderContext struct {
	writer      io.Writer
	renderer    *lipgloss.Renderer
	atmosConfig *schema.AtmosConfiguration
	styles      *helpStyles
}

// createHelpStyles creates the color styles for help output using theme-aware colors.
func createHelpStyles(renderer *lipgloss.Renderer) helpStyles {
	defer perf.Track(nil, "cmd.createHelpStyles")()

	// Get theme-aware styles.
	themeStyles := theme.GetCurrentStyles()

	// Apply renderer to theme styles so they use the correct color profile.
	return helpStyles{
		heading:     renderer.NewStyle().Foreground(themeStyles.Help.Heading.GetForeground()).Bold(true),
		commandName: renderer.NewStyle().Foreground(themeStyles.Help.CommandName.GetForeground()).Bold(true),
		commandDesc: renderer.NewStyle().Foreground(themeStyles.Help.CommandDesc.GetForeground()),
		flagName:    renderer.NewStyle().Foreground(themeStyles.Help.FlagName.GetForeground()),
		flagDesc:    renderer.NewStyle().Foreground(themeStyles.Help.FlagDesc.GetForeground()),
		muted:       renderer.NewStyle().Foreground(themeStyles.Muted.GetForeground()),
	}
}

// printLogoAndVersion prints the Atmos logo and version information.
func printLogoAndVersion(w io.Writer, styles *helpStyles) {
	defer perf.Track(nil, "cmd.printLogoAndVersion")()

	fmt.Fprintln(w)
	_ = tuiUtils.PrintStyledTextToSpecifiedOutput(w, "ATMOS")

	versionText := styles.muted.Render(version.Version)
	osArchText := styles.muted.Render(fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH))
	versionInfo := fmt.Sprintf("👽 %s %s", versionText, osArchText)
	fmt.Fprintln(w, versionInfo)
	fmt.Fprintln(w)
}

// printDescription prints the command description.
func printDescription(w io.Writer, cmd *cobra.Command, styles *helpStyles) {
	defer perf.Track(nil, "cmd.printDescription")()

	// Print experimental badge if command is experimental.
	if isExperimentalCommand(cmd) {
		badge := ui.FormatExperimentalBadge()
		fmt.Fprintln(w, badge)
		fmt.Fprintln(w)
	}

	var desc string
	switch {
	case cmd.Long != "":
		desc = cmd.Long
	case cmd.Short != "":
		desc = cmd.Short
	default:
		return
	}

	// Use markdown rendering to respect terminal width and wrapping settings.
	// This ensures long descriptions wrap properly based on screen width.
	rendered := renderMarkdownDescription(desc)
	styled := styles.commandDesc.Render(rendered)
	// Lipgloss pads multi-line strings to uniform width. Trim trailing whitespace from each line.
	styled = atmosansi.TrimLinesRight(styled)
	fmt.Fprintln(w, styled)
	fmt.Fprintln(w)
}

// printUsageSection prints the usage section.
func printUsageSection(w io.Writer, cmd *cobra.Command, renderer *lipgloss.Renderer, styles *helpStyles) {
	defer perf.Track(nil, "cmd.printUsageSection")()

	fmt.Fprintln(w, styles.heading.Render("USAGE"))
	fmt.Fprintln(w)
	var usageContent strings.Builder
	if cmd.Runnable() {
		fmt.Fprintf(&usageContent, "$ %s\n", cmd.UseLine())
	}
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintf(&usageContent, "$ %s", cmd.CommandPath()+" [sub-command] [flags]")
	}
	if usageContent.Len() > 0 {
		usageText := strings.TrimRight(usageContent.String(), "\n")
		termWidth := getTerminalWidth()
		rendered := markdown.RenderCodeBlock(renderer, usageText, termWidth)
		fmt.Fprintln(w, rendered)
	}
	fmt.Fprintln(w)
}

// printAliases prints command aliases.
func printAliases(w io.Writer, cmd *cobra.Command, styles *helpStyles) {
	defer perf.Track(nil, "cmd.printAliases")()

	if len(cmd.Aliases) > 0 {
		fmt.Fprintln(w, styles.heading.Render("ALIASES"))
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  %s\n", styles.commandDesc.Render(cmd.NameAndAliases()))
		fmt.Fprintln(w)
	}
}

// renderMarkdownDescription renders a description string as Markdown using the UI formatter.
// Uses the global ui.Format which has theme integration and automatic degradation.
func renderMarkdownDescription(desc string) string {
	if ui.Format == nil {
		return desc
	}

	rendered, err := ui.Format.Markdown(desc)
	if err != nil {
		return desc
	}

	return strings.TrimSpace(rendered)
}

// printSubcommandAliases prints subcommand aliases.
func printSubcommandAliases(ctx *helpRenderContext, cmd *cobra.Command) {
	defer perf.Track(nil, "cmd.printSubcommandAliases")()

	hasAliases := false
	for _, c := range cmd.Commands() {
		if c.IsAvailableCommand() && len(c.Aliases) > 0 {
			hasAliases = true
			break
		}
	}
	if !hasAliases {
		return
	}

	fmt.Fprintln(ctx.writer, ctx.styles.heading.Render("SUBCOMMAND ALIASES"))
	fmt.Fprintln(ctx.writer)
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || len(c.Aliases) == 0 {
			continue
		}
		name := ctx.styles.commandName.Render(fmt.Sprintf("%-15s", c.Aliases[0]))

		// Render description as Markdown (like command descriptions) with backticks instead of quotes.
		desc := fmt.Sprintf("Alias of `%s %s` command", cmd.Name(), c.Name())
		desc = renderMarkdownDescription(desc)

		fmt.Fprintf(ctx.writer, "      %s  %s\n", name, desc)
	}
	fmt.Fprintln(ctx.writer)
}

// printExamples prints command examples.
func printExamples(w io.Writer, cmd *cobra.Command, renderer *lipgloss.Renderer, styles *helpStyles) {
	defer perf.Track(nil, "cmd.printExamples")()

	if cmd.Example == "" {
		return
	}

	fmt.Fprintln(w, styles.heading.Render("EXAMPLES"))
	fmt.Fprintln(w)

	exampleText := strings.TrimSpace(cmd.Example)
	exampleText = strings.ReplaceAll(exampleText, "```shell", "")
	exampleText = strings.ReplaceAll(exampleText, "```", "")

	termWidth := getTerminalWidth()
	rendered := markdown.RenderCodeBlock(renderer, exampleText, termWidth)

	fmt.Fprintln(w, rendered)
	fmt.Fprintln(w)
}

// isCommandAvailable checks if a command should be shown in help output.
func isCommandAvailable(cmd *cobra.Command) bool {
	return cmd.IsAvailableCommand() || cmd.Name() == "help"
}

// isConfigAlias checks if a command is a CLI config alias.
func isConfigAlias(cmd *cobra.Command) bool {
	if cmd.Annotations == nil {
		return false
	}
	_, ok := cmd.Annotations[annotationConfigAlias]
	return ok
}

// isCustomCommand checks if a command was created from Atmos command configuration.
func isCustomCommand(cmd *cobra.Command) bool {
	return cmd.Annotations != nil && cmd.Annotations[annotationCustomCommand] == annotationValueTrue
}

// calculateCommandWidth calculates the display width of a command name including type suffix.
// If parentExperimental is true, experimental badges won't be shown on subcommands.
func calculateCommandWidth(cmd *cobra.Command, parentExperimental bool) int {
	width := len(cmd.Name())
	if cmd.HasAvailableSubCommands() {
		width += len(" [command]")
	}
	// Account for experimental badge if present and parent is not already experimental.
	if isExperimentalCommand(cmd) && !parentExperimental {
		width += len(" [EXPERIMENTAL]")
	}
	return width
}

// calculateMaxCommandWidth finds the maximum command name width for alignment.
// Config aliases are excluded from this calculation since they're shown in a separate section.
// If parentExperimental is true, experimental badges won't be included in width calculations.
func calculateMaxCommandWidth(commands []*cobra.Command, parentExperimental bool, customCommands bool) int {
	maxWidth := 0
	for _, c := range commands {
		if !isCommandAvailable(c) || isConfigAlias(c) || isCustomCommand(c) != customCommands {
			continue
		}
		width := calculateCommandWidth(c, parentExperimental)
		if width > maxWidth {
			maxWidth = width
		}
	}
	return maxWidth
}

// getExperimentalBadge returns the styled and plain badge strings if command is experimental.
// Returns empty strings if parent is already experimental or command is not experimental.
func getExperimentalBadge(cmd *cobra.Command, parentExperimental bool) (styled, plain string) {
	if isExperimentalCommand(cmd) && !parentExperimental {
		return " " + ui.FormatExperimentalBadge(), " [EXPERIMENTAL]"
	}
	return "", ""
}

// formatCommandLine formats a single command line with proper padding and styling.
func formatCommandLine(ctx *helpRenderContext, cmd *cobra.Command, maxWidth int, mdRenderer *markdown.Renderer, parentExperimental bool) {
	cmdName := cmd.Name()
	cmdTypePlain, cmdTypeStyled := "", ""
	if cmd.HasAvailableSubCommands() {
		cmdTypePlain = " [command]"
		cmdTypeStyled = " " + ctx.styles.flagName.Render("[command]")
	}

	experimentalBadge, experimentalBadgePlain := getExperimentalBadge(cmd, parentExperimental)
	padding := maxWidth - len(cmdName) - len(cmdTypePlain) - len(experimentalBadgePlain)

	// Calculate description column position and width.
	descColStart := commandListLeftPad + maxWidth + commandDescriptionSpacing
	descWidth := getTerminalWidth() - descColStart
	if descWidth < minDescriptionWidth {
		descWidth = minDescriptionWidth
	}

	// Write command name and badges.
	fmt.Fprint(ctx.writer, strings.Repeat(spaceChar, commandListLeftPad))
	fmt.Fprint(ctx.writer, ctx.styles.commandName.Render(cmdName))
	fmt.Fprint(ctx.writer, cmdTypeStyled)
	fmt.Fprint(ctx.writer, experimentalBadge)
	fmt.Fprint(ctx.writer, strings.Repeat(spaceChar, padding+commandDescriptionSpacing))

	// Render description with word wrap and markdown.
	wrapped := wordwrap.String(cmd.Short, descWidth)
	if mdRenderer != nil {
		if rendered, err := mdRenderer.RenderWithoutWordWrap(wrapped); err == nil {
			wrapped = strings.TrimSpace(rendered)
		}
	}

	// Write description lines.
	lines := strings.Split(wrapped, "\n")
	if len(lines) > 0 {
		fmt.Fprintf(ctx.writer, "%s\n", ctx.styles.commandDesc.Render(lines[0]))
	}
	for i := 1; i < len(lines); i++ {
		fmt.Fprintf(ctx.writer, "%s%s\n", strings.Repeat(spaceChar, descColStart), ctx.styles.commandDesc.Render(lines[i]))
	}
}

type commandSectionOptions struct {
	heading            string
	customCommands     bool
	parentExperimental bool
	mdRenderer         *markdown.Renderer
}

// printCommandSection prints one command-origin section.
func printCommandSection(ctx *helpRenderContext, cmd *cobra.Command, opts commandSectionOptions) {
	maxCmdWidth := calculateMaxCommandWidth(cmd.Commands(), opts.parentExperimental, opts.customCommands)
	if maxCmdWidth == 0 {
		return
	}

	fmt.Fprintln(ctx.writer, ctx.styles.heading.Render(opts.heading))
	fmt.Fprintln(ctx.writer)

	for _, c := range cmd.Commands() {
		if !isCommandAvailable(c) || isConfigAlias(c) || isCustomCommand(c) != opts.customCommands {
			continue
		}
		formatCommandLine(ctx, c, maxCmdWidth, opts.mdRenderer, opts.parentExperimental)
	}
	fmt.Fprintln(ctx.writer)
}

// printAvailableCommands prints the list of available subcommands.
func printAvailableCommands(ctx *helpRenderContext, cmd *cobra.Command) {
	defer perf.Track(nil, "cmd.printAvailableCommands")()

	if !cmd.HasAvailableSubCommands() {
		return
	}

	// Check if the parent command is experimental.
	// If so, subcommands don't need to repeat the badge since it's shown at the top.
	parentExperimental := isExperimentalCommand(cmd)

	// Create markdown renderer for command descriptions (same approach as flag rendering).
	var mdRenderer *markdown.Renderer
	if ctx.atmosConfig != nil {
		mdRenderer, _ = markdown.NewTerminalMarkdownRenderer(*ctx.atmosConfig)
	}

	printCommandSection(ctx, cmd, commandSectionOptions{
		heading:            "BUILT-IN COMMANDS",
		customCommands:     false,
		parentExperimental: parentExperimental,
		mdRenderer:         mdRenderer,
	})
	printCommandSection(ctx, cmd, commandSectionOptions{
		heading:            "CUSTOM COMMANDS",
		customCommands:     true,
		parentExperimental: parentExperimental,
		mdRenderer:         mdRenderer,
	})
}

// getConfigAliases returns all available config alias commands.
func getConfigAliases(cmd *cobra.Command) []*cobra.Command {
	var aliases []*cobra.Command
	for _, c := range cmd.Commands() {
		if c.IsAvailableCommand() && isConfigAlias(c) {
			aliases = append(aliases, c)
		}
	}
	return aliases
}

// printConfigAliases prints CLI config aliases in a dedicated section.
func printConfigAliases(ctx *helpRenderContext, cmd *cobra.Command) {
	defer perf.Track(nil, "cmd.printConfigAliases")()

	aliases := getConfigAliases(cmd)
	if len(aliases) == 0 {
		return
	}

	fmt.Fprintln(ctx.writer, ctx.styles.heading.Render("ALIASES"))
	fmt.Fprintln(ctx.writer)

	// Calculate max width for alignment.
	maxWidth := 0
	for _, c := range aliases {
		if len(c.Name()) > maxWidth {
			maxWidth = len(c.Name())
		}
	}

	for _, c := range aliases {
		name := ctx.styles.commandName.Render(fmt.Sprintf("%-*s", maxWidth, c.Name()))
		// c.Short contains "alias for `command`".
		desc := renderMarkdownDescription(c.Short)
		fmt.Fprintf(ctx.writer, "      %s  %s\n", name, desc)
	}
	fmt.Fprintln(ctx.writer)
}

// printFlags prints command flags.
func printFlags(w io.Writer, cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration, styles *helpStyles) {
	defer perf.Track(nil, "cmd.printFlags")()

	termWidth := getTerminalWidth()

	if cmd.HasAvailableLocalFlags() {
		fmt.Fprintln(w, styles.heading.Render("FLAGS"))
		fmt.Fprintln(w)
		renderFlags(w, cmd.LocalFlags(), styles.commandName, styles.flagName, styles.flagDesc, termWidth, atmosConfig)
		fmt.Fprintln(w)
	}

	if cmd.HasAvailableInheritedFlags() {
		fmt.Fprintln(w, styles.heading.Render("GLOBAL FLAGS"))
		fmt.Fprintln(w)
		renderFlags(w, cmd.InheritedFlags(), styles.commandName, styles.flagName, styles.flagDesc, termWidth, atmosConfig)
		fmt.Fprintln(w)
	}
}

// printCompatibilityFlags prints terraform compatibility flags if applicable.
// These are pass-through flags that are forwarded to the underlying terraform/tofu command.
func printCompatibilityFlags(w io.Writer, cmd *cobra.Command, styles *helpStyles) {
	defer perf.Track(nil, "cmd.printCompatibilityFlags")()

	// Find the top-level command (e.g., "terraform") to determine if this command has compat flags.
	providerName := findProviderName(cmd)
	if providerName == "" {
		return
	}

	// Get subcommand-specific compatibility flags.
	flags := internal.GetSubcommandCompatFlags(providerName, cmd.Name())
	if len(flags) == 0 {
		return
	}

	fmt.Fprintln(w, styles.heading.Render("COMPATIBILITY FLAGS"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  These flags are passed through to the underlying terraform/tofu command.")
	fmt.Fprintln(w)

	renderCompatFlags(w, flags, &styles.flagName, &styles.flagName, &styles.flagDesc)
	fmt.Fprintln(w)
}

// findProviderName walks up the command tree to find the top-level command name.
// For example, for "atmos terraform apply", this returns "terraform".
func findProviderName(cmd *cobra.Command) string {
	defer perf.Track(nil, "cmd.findProviderName")()

	// Walk up to find the first non-root parent.
	for c := cmd; c != nil; c = c.Parent() {
		parent := c.Parent()
		if parent != nil && parent.Parent() == nil {
			// c's parent is root, so c is the top-level command.
			return c.Name()
		}
	}
	return ""
}

// renderCompatFlags renders compatibility flags with proper styling and alignment.
// FlagStyle is used for the flag name (cyan), argTypeStyle for any type annotations, descStyle for descriptions.
func renderCompatFlags(w io.Writer, flags map[string]compat.CompatibilityFlag, flagStyle, argTypeStyle, descStyle *lipgloss.Style) {
	defer perf.Track(nil, "cmd.renderCompatFlags")()

	// Collect and sort flag names for consistent output.
	type flagEntry struct {
		name        string
		description string
	}
	entries := make([]flagEntry, 0, len(flags))
	for name, flag := range flags {
		entries = append(entries, flagEntry{name: name, description: flag.Description})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})

	// Calculate max flag name length for alignment.
	maxLen := 0
	for _, entry := range entries {
		if len(entry.name) > maxLen {
			maxLen = len(entry.name)
		}
	}

	// Calculate available width for description.
	termWidth := getTerminalWidth()
	// Layout: commandListLeftPad padding + flag name + commandDescriptionSpacing + description
	descColStart := commandListLeftPad + maxLen + commandDescriptionSpacing
	descWidth := termWidth - descColStart
	if descWidth < minDescriptionWidth {
		descWidth = minDescriptionWidth
	}

	// Render each flag.
	// Note: argTypeStyle is available but not currently used since compat flags don't have type annotations.
	_ = argTypeStyle

	for _, entry := range entries {
		padding := maxLen - len(entry.name) + 2

		// Style the flag name (cyan, like FLAGS section).
		styledName := flagStyle.Render(entry.name)

		// Wrap and render description.
		wrapped := wordwrap.String(entry.description, descWidth)
		lines := strings.Split(wrapped, "\n")

		// Print first line.
		fmt.Fprintf(w, "      %s%s%s\n", styledName, strings.Repeat(" ", padding), descStyle.Render(lines[0]))

		// Print continuation lines with proper indentation.
		if len(lines) > 1 {
			indentStr := strings.Repeat(" ", descColStart)
			for i := 1; i < len(lines); i++ {
				fmt.Fprintf(w, "%s%s\n", indentStr, descStyle.Render(lines[i]))
			}
		}
	}
}

// applyColoredHelpTemplate applies a colored help template to the command.
// This approach ensures colors work in both interactive terminals and redirected output (screengrabs).
// Colors are automatically enabled when ATMOS_FORCE_COLOR, CLICOLOR_FORCE, or FORCE_COLOR is set.
//
// Color detection is the single shared pipeline: init() runs setupColorProfileFromEnv()
// and ui.InitFormatter() from process-level facts (env vars, os.Args, fd TTY state) with
// no atmos.yaml required; configureEarlyColorProfile() refines from the parsed flags; the
// renderer then inherits the resulting global profile via ui.NewRenderer(). Help must
// render correctly even when the Atmos configuration is missing or invalid.
func applyColoredHelpTemplate(cmd *cobra.Command) {
	applyColoredHelpTemplateForTopic(cmd, helpTopicRequest{valid: true})
}

func applyColoredHelpTemplateForTopic(cmd *cobra.Command, topic helpTopicRequest) {
	defer perf.Track(nil, "cmd.applyColoredHelpTemplateForTopic")()

	// Refine the global color profile from the parsed --no-color/--force-color flags.
	configureEarlyColorProfile(cmd)

	// Bind a renderer to the help writer using the globally detected profile.
	// The root help function starts explicit cast recording before help renders;
	// cmd's normal masked output writer records the rendered help.
	renderer := ui.NewRenderer(cmd.OutOrStdout())
	log.Debug("Help renderer configured", "profile", renderer.ColorProfile())

	// Create help styles.
	styles := createHelpStyles(renderer)

	// Load Atmos configuration for markdown rendering.
	// Reuse existing atmosConfig from root/Execute if available, otherwise load a minimal config.
	// Use processStacks=false since help rendering only needs terminal/docs settings.
	var helpAtmosConfig *schema.AtmosConfiguration
	if atmosConfig.BasePath != "" {
		// atmosConfig already loaded by root command.
		helpAtmosConfig = &atmosConfig
	} else {
		// Load minimal config without processing stacks.
		config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			// If config loading fails, use a minimal zero-value config.
			config = schema.AtmosConfiguration{}
		}
		helpAtmosConfig = &config
	}

	// Create help render context.
	ctx := &helpRenderContext{
		writer:      cmd.OutOrStdout(),
		renderer:    renderer,
		atmosConfig: helpAtmosConfig,
		styles:      &styles,
	}

	// Set custom help function.
	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		printHelpForTopic(ctx, c, topic)
	})
}

// printFooter prints the help footer message.
func printFooter(w io.Writer, cmd *cobra.Command, styles *helpStyles) {
	defer perf.Track(nil, "cmd.printFooter")()

	if !cmd.HasAvailableSubCommands() {
		return
	}

	usageMsg := fmt.Sprintf("Use `%s [command] --help` for more information about a command.", cmd.CommandPath())
	// Use renderMarkdownDescription to respect terminal width and wrapping settings.
	// This ensures the footer wraps properly based on screen width.
	usageMsg = renderMarkdownDescription(usageMsg)
	fmt.Fprintf(w, "\n%s\n", styles.muted.Render(usageMsg))
}
