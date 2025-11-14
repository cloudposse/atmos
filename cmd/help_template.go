package cmd

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	markdown "github.com/cloudposse/atmos/pkg/ui/markdown"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/version"
)

// Environment variable names for color control.
const (
	envAtmosForceColor = "ATMOS_FORCE_COLOR"
	envForceColor      = "FORCE_COLOR"
	envCliColorForce   = "CLICOLOR_FORCE"
	envNoColor         = "NO_COLOR"
	envAtmosDebugColor = "ATMOS_DEBUG_COLORS"
	envTerm            = "TERM"
	envColorTerm       = "COLORTERM"
)

// String constants for environment variable values.
const (
	valueOne  = "1"
	valueZero = "0"
)

// colorConfig holds the color detection and environment variable configuration.
type colorConfig struct {
	forceColor         bool
	explicitlyDisabled bool
	debugColors        bool
}

// writerConfig holds the writer and renderer configuration.
type writerConfig struct {
	writer   io.Writer
	renderer *lipgloss.Renderer
	profile  colorprofile.Profile
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

// parseBoolLikeForceColor parses a FORCE_COLOR-style environment variable value.
// Returns (isTruthy, isFalsy) to distinguish between truthy and falsy values.
// Truthy values: "1", "true", "2", "3", "yes", "on", "always".
// Falsy values: "0", "false", "no", "off".
func parseBoolLikeForceColor(val string) (isTruthy bool, isFalsy bool) {
	if val == "" {
		return false, false
	}

	v := strings.ToLower(strings.TrimSpace(val))
	if v == "" {
		return false, false
	}

	// Check truthy values
	truthyValues := []string{"1", "true", "2", "3", "yes", "on", "always"}
	for _, truthy := range truthyValues {
		if v == truthy {
			return true, false
		}
	}

	// Check falsy values
	falsyValues := []string{"0", "false", "no", "off"}
	for _, falsy := range falsyValues {
		if v == falsy {
			return false, true
		}
	}

	// Value is set but not recognized - treat as neither truthy nor falsy
	return false, false
}

// isTruthy checks if a string represents a truthy value.
func isTruthy(val string) bool {
	truthy, _ := parseBoolLikeForceColor(val)
	return truthy
}

// isFalsy checks if a string represents a falsy value.
func isFalsy(val string) bool {
	_, falsy := parseBoolLikeForceColor(val)
	return falsy
}

// detectColorConfig detects and configures color settings based on environment variables.
func detectColorConfig() colorConfig {
	defer perf.Track(nil, "cmd.detectColorConfig")()

	// Bind environment variables for color control.
	_ = viper.BindEnv(envAtmosForceColor)
	_ = viper.BindEnv(envCliColorForce)
	_ = viper.BindEnv(envForceColor)
	_ = viper.BindEnv(envNoColor)
	_ = viper.BindEnv(envAtmosDebugColor)

	// Check ATMOS_FORCE_COLOR first, then fallback to standard env vars.
	atmosForceColor := viper.GetString(envAtmosForceColor)
	cliColorForce := viper.GetString(envCliColorForce)
	forceColorEnv := viper.GetString(envForceColor)

	// Determine final forceColor value.
	explicitlyDisabled := isFalsy(atmosForceColor) || isFalsy(cliColorForce) || isFalsy(forceColorEnv)
	forceColor := false
	if !explicitlyDisabled {
		forceColor = isTruthy(atmosForceColor) || isTruthy(cliColorForce) || isTruthy(forceColorEnv)
	}

	// Ensure standard env vars are set for ALL color libraries.
	if explicitlyDisabled {
		os.Setenv(envNoColor, valueOne)
		os.Setenv(envForceColor, valueZero)
		os.Setenv(envCliColorForce, valueZero)
	} else if forceColor {
		os.Unsetenv(envNoColor)
		if viper.GetString(envForceColor) == "" {
			os.Setenv(envForceColor, valueOne)
		}
		if viper.GetString(envCliColorForce) == "" {
			os.Setenv(envCliColorForce, valueOne)
		}
	}

	debugColors := viper.GetString(envAtmosDebugColor) != ""

	return colorConfig{
		forceColor:         forceColor,
		explicitlyDisabled: explicitlyDisabled,
		debugColors:        debugColors,
	}
}

// printColorDebugInfo prints debug information about color detection.
func printColorDebugInfo(profileDetector *colorprofile.Writer, config colorConfig) {
	fmt.Fprintf(os.Stderr, "\n[DEBUG] Color Detection:\n")
	fmt.Fprintf(os.Stderr, "  Detected Profile: %v\n", profileDetector.Profile)
	fmt.Fprintf(os.Stderr, "  ATMOS_FORCE_COLOR: %s\n", viper.GetString(envAtmosForceColor))
	fmt.Fprintf(os.Stderr, "  FORCE_COLOR: %s\n", viper.GetString(envForceColor))
	fmt.Fprintf(os.Stderr, "  CLICOLOR_FORCE: %s\n", viper.GetString(envCliColorForce))
	fmt.Fprintf(os.Stderr, "  NO_COLOR: %s\n", viper.GetString(envNoColor))
	fmt.Fprintf(os.Stderr, "  TERM: %s\n", viper.GetString(envTerm))
	fmt.Fprintf(os.Stderr, "  COLORTERM: %s\n", viper.GetString(envColorTerm))
	fmt.Fprintf(os.Stderr, "  forceColor: %v\n", config.forceColor)
	fmt.Fprintf(os.Stderr, "  explicitlyDisabled: %v\n", config.explicitlyDisabled)
}

// configureDisabledColorWriter creates a writer with colors explicitly disabled.
func configureDisabledColorWriter(out io.Writer, debugColors bool) (io.Writer, colorprofile.Profile, *lipgloss.Renderer) {
	colorW := colorprofile.NewWriter(out, os.Environ())
	colorW.Profile = colorprofile.Ascii
	renderer := lipgloss.NewRenderer(colorW)
	renderer.SetColorProfile(termenv.Ascii)

	if debugColors {
		fmt.Fprintf(os.Stderr, "  Mode: Explicitly Disabled\n")
		fmt.Fprintf(os.Stderr, "  Final Profile: Ascii\n")
	}

	return colorW, colorprofile.Ascii, renderer
}

// configureForcedColorWriter creates a writer with colors forced on.
func configureForcedColorWriter(out io.Writer, debugColors bool) (io.Writer, colorprofile.Profile, *lipgloss.Renderer) {
	profile := colorprofile.ANSI256
	termOut := termenv.NewOutput(out, termenv.WithProfile(termenv.ANSI256))
	termenv.SetDefaultOutput(termOut)
	renderer := lipgloss.NewRenderer(termOut, termenv.WithProfile(termenv.ANSI256))

	if debugColors {
		fmt.Fprintf(os.Stderr, "  Mode: Force Color (pipe-safe)\n")
		fmt.Fprintf(os.Stderr, "  Final Profile: ANSI256 (forced)\n")
		fmt.Fprintf(os.Stderr, "  Renderer: Created with termenv.Output ANSI256 as writer\n")
		fmt.Fprintf(os.Stderr, "  Renderer ColorProfile: %v\n", renderer.ColorProfile())
		fmt.Fprintf(os.Stderr, "  Global termenv DefaultOutput profile: %v\n", termenv.DefaultOutput().ColorProfile())
	}

	return out, profile, renderer
}

// setRendererProfileForAutoDetect configures the renderer based on the detected color profile.
func setRendererProfileForAutoDetect(renderer *lipgloss.Renderer, profile colorprofile.Profile, debugColors bool) {
	switch profile {
	case colorprofile.TrueColor:
		renderer.SetColorProfile(termenv.TrueColor)
		if debugColors {
			fmt.Fprintf(os.Stderr, "  Renderer: Auto-detect, set to TrueColor\n")
		}
	case colorprofile.ANSI256:
		renderer.SetColorProfile(termenv.ANSI256)
		if debugColors {
			fmt.Fprintf(os.Stderr, "  Renderer: Auto-detect, set to ANSI256\n")
		}
	case colorprofile.ANSI:
		renderer.SetColorProfile(termenv.ANSI)
		if debugColors {
			fmt.Fprintf(os.Stderr, "  Renderer: Auto-detect, set to ANSI\n")
		}
	case colorprofile.Ascii:
		renderer.SetColorProfile(termenv.Ascii)
		if debugColors {
			fmt.Fprintf(os.Stderr, "  Renderer: Auto-detect, set to Ascii\n")
		}
	}
}

// configureAutoDetectColorWriter creates a writer with auto-detected color support.
func configureAutoDetectColorWriter(out io.Writer, detectedProfile colorprofile.Profile, debugColors bool) (io.Writer, colorprofile.Profile, *lipgloss.Renderer) {
	colorW := colorprofile.NewWriter(out, os.Environ())
	colorW.Profile = detectedProfile
	renderer := lipgloss.NewRenderer(colorW)

	setRendererProfileForAutoDetect(renderer, colorW.Profile, debugColors)

	if debugColors {
		fmt.Fprintf(os.Stderr, "  Mode: Auto-detect\n")
		fmt.Fprintf(os.Stderr, "  Final Profile: %v\n", colorW.Profile)
	}

	return colorW, colorW.Profile, renderer
}

// configureWriter creates and configures the writer and renderer based on color settings.
func configureWriter(cmd *cobra.Command, config colorConfig) writerConfig {
	defer perf.Track(nil, "cmd.configureWriter")()

	profileDetector := colorprofile.NewWriter(os.Stdout, os.Environ())

	if config.debugColors {
		printColorDebugInfo(profileDetector, config)
	}

	var w io.Writer
	var profile colorprofile.Profile
	var renderer *lipgloss.Renderer

	switch {
	case config.explicitlyDisabled:
		w, profile, renderer = configureDisabledColorWriter(cmd.OutOrStdout(), config.debugColors)
	case config.forceColor:
		w, profile, renderer = configureForcedColorWriter(cmd.OutOrStdout(), config.debugColors)
	default:
		w, profile, renderer = configureAutoDetectColorWriter(cmd.OutOrStdout(), profileDetector.Profile, config.debugColors)
	}

	if config.debugColors {
		fmt.Fprintf(os.Stderr, "  Renderer Color Profile: %v\n", renderer.ColorProfile())
		fmt.Fprintf(os.Stderr, "  Renderer Has Dark Background: %v\n\n", renderer.HasDarkBackground())
	}

	renderer.SetHasDarkBackground(true)

	return writerConfig{
		writer:   w,
		renderer: renderer,
		profile:  profile,
	}
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
	fmt.Fprintln(w, styles.commandDesc.Render(rendered))
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

// calculateCommandWidth calculates the display width of a command name including type suffix.
func calculateCommandWidth(cmd *cobra.Command) int {
	width := len(cmd.Name())
	if cmd.HasAvailableSubCommands() {
		width += len(" [command]")
	}
	return width
}

// calculateMaxCommandWidth finds the maximum command name width for alignment.
func calculateMaxCommandWidth(commands []*cobra.Command) int {
	maxWidth := 0
	for _, c := range commands {
		if !isCommandAvailable(c) {
			continue
		}
		width := calculateCommandWidth(c)
		if width > maxWidth {
			maxWidth = width
		}
	}
	return maxWidth
}

// formatCommandLine formats a single command line with proper padding and styling.
func formatCommandLine(ctx *helpRenderContext, cmd *cobra.Command, maxWidth int) {
	cmdName := cmd.Name()
	cmdTypePlain := ""
	cmdTypeStyled := ""
	if cmd.HasAvailableSubCommands() {
		cmdTypePlain = " [command]"
		cmdTypeStyled = " " + ctx.styles.flagName.Render("[command]")
	}

	padding := maxWidth - len(cmdName) - len(cmdTypePlain)

	fmt.Fprint(ctx.writer, "      ")
	fmt.Fprint(ctx.writer, ctx.styles.commandName.Render(cmdName))
	fmt.Fprint(ctx.writer, cmdTypeStyled)
	fmt.Fprint(ctx.writer, strings.Repeat(" ", padding))

	// Render description as Markdown (like flags do).
	desc := renderMarkdownDescription(cmd.Short)
	fmt.Fprintf(ctx.writer, "  %s\n", desc)
}

// printAvailableCommands prints the list of available subcommands.
func printAvailableCommands(ctx *helpRenderContext, cmd *cobra.Command) {
	defer perf.Track(nil, "cmd.printAvailableCommands")()

	if !cmd.HasAvailableSubCommands() {
		return
	}

	fmt.Fprintln(ctx.writer, ctx.styles.heading.Render("AVAILABLE COMMANDS"))
	fmt.Fprintln(ctx.writer)

	maxCmdWidth := calculateMaxCommandWidth(cmd.Commands())

	for _, c := range cmd.Commands() {
		if !isCommandAvailable(c) {
			continue
		}
		formatCommandLine(ctx, c, maxCmdWidth)
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

// applyColoredHelpTemplate applies a colored help template to the command.
// This approach ensures colors work in both interactive terminals and redirected output (screengrabs).
// Colors are automatically enabled when ATMOS_FORCE_COLOR, CLICOLOR_FORCE, or FORCE_COLOR is set.
func applyColoredHelpTemplate(cmd *cobra.Command) {
	defer perf.Track(nil, "cmd.applyColoredHelpTemplate")()

	// Detect and configure color settings.
	colorConf := detectColorConfig()

	// Configure writer and renderer.
	writerConf := configureWriter(cmd, colorConf)

	// Create help styles.
	styles := createHelpStyles(writerConf.renderer)

	// Load Atmos configuration for markdown rendering.
	atmosConfig, _ := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)

	// Create help render context.
	ctx := &helpRenderContext{
		writer:      writerConf.writer,
		renderer:    writerConf.renderer,
		atmosConfig: &atmosConfig,
		styles:      &styles,
	}

	// Set custom help function.
	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		printLogoAndVersion(ctx.writer, ctx.styles)
		printDescription(ctx.writer, c, ctx.styles)
		printUsageSection(ctx.writer, c, ctx.renderer, ctx.styles)
		printAliases(ctx.writer, c, ctx.styles)
		printSubcommandAliases(ctx, c)
		printExamples(ctx.writer, c, ctx.renderer, ctx.styles)
		printAvailableCommands(ctx, c)
		printFlags(ctx.writer, c, ctx.atmosConfig, ctx.styles)
		printFooter(ctx.writer, c, ctx.styles)
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
