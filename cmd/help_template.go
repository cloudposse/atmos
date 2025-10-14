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
	markdown "github.com/cloudposse/atmos/pkg/ui/markdown"
	"github.com/cloudposse/atmos/pkg/version"
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
type helpStyles struct {
	cyan       lipgloss.Style
	green      lipgloss.Style
	gray       lipgloss.Style
	dimmedGray lipgloss.Style
	lightGray  lipgloss.Style
	darkGray   lipgloss.Style
}

// isTruthy checks if a string represents a truthy value.
func isTruthy(val string) bool {
	if val == "" {
		return false
	}
	v := strings.ToLower(strings.TrimSpace(val))
	return v == "1" || v == "true"
}

// isFalsy checks if a string represents a falsy value.
func isFalsy(val string) bool {
	if val == "" {
		return false
	}
	v := strings.ToLower(strings.TrimSpace(val))
	return v == "0" || v == "false"
}

// detectColorConfig detects and configures color settings based on environment variables.
func detectColorConfig() colorConfig {
	defer perf.Track(nil, "cmd.detectColorConfig")()

	// Bind environment variables for color control.
	_ = viper.BindEnv("ATMOS_FORCE_COLOR")
	_ = viper.BindEnv("CLICOLOR_FORCE")
	_ = viper.BindEnv("FORCE_COLOR")
	_ = viper.BindEnv("NO_COLOR")
	_ = viper.BindEnv("ATMOS_DEBUG_COLORS")

	// Check ATMOS_FORCE_COLOR first, then fallback to standard env vars.
	atmosForceColor := viper.GetString("ATMOS_FORCE_COLOR")
	cliColorForce := viper.GetString("CLICOLOR_FORCE")
	forceColorEnv := viper.GetString("FORCE_COLOR")

	// Determine final forceColor value.
	explicitlyDisabled := isFalsy(atmosForceColor) || isFalsy(cliColorForce) || isFalsy(forceColorEnv)
	forceColor := false
	if !explicitlyDisabled {
		forceColor = isTruthy(atmosForceColor) || isTruthy(cliColorForce) || isTruthy(forceColorEnv)
	}

	// Ensure standard env vars are set for ALL color libraries.
	if explicitlyDisabled {
		os.Setenv("NO_COLOR", "1")
		os.Setenv("FORCE_COLOR", "0")
		os.Setenv("CLICOLOR_FORCE", "0")
	} else if forceColor {
		os.Unsetenv("NO_COLOR")
		if viper.GetString("FORCE_COLOR") == "" {
			os.Setenv("FORCE_COLOR", "1")
		}
		if viper.GetString("CLICOLOR_FORCE") == "" {
			os.Setenv("CLICOLOR_FORCE", "1")
		}
	}

	debugColors := viper.GetString("ATMOS_DEBUG_COLORS") != ""

	return colorConfig{
		forceColor:         forceColor,
		explicitlyDisabled: explicitlyDisabled,
		debugColors:        debugColors,
	}
}

// configureWriter creates and configures the writer and renderer based on color settings.
func configureWriter(cmd *cobra.Command, config colorConfig) writerConfig {
	defer perf.Track(nil, "cmd.configureWriter")()

	profileDetector := colorprofile.NewWriter(os.Stdout, os.Environ())

	if config.debugColors {
		fmt.Fprintf(os.Stderr, "\n[DEBUG] Color Detection:\n")
		fmt.Fprintf(os.Stderr, "  Detected Profile: %v\n", profileDetector.Profile)
		fmt.Fprintf(os.Stderr, "  ATMOS_FORCE_COLOR: %s\n", viper.GetString("ATMOS_FORCE_COLOR"))
		fmt.Fprintf(os.Stderr, "  FORCE_COLOR: %s\n", viper.GetString("FORCE_COLOR"))
		fmt.Fprintf(os.Stderr, "  CLICOLOR_FORCE: %s\n", viper.GetString("CLICOLOR_FORCE"))
		fmt.Fprintf(os.Stderr, "  NO_COLOR: %s\n", viper.GetString("NO_COLOR"))
		fmt.Fprintf(os.Stderr, "  TERM: %s\n", viper.GetString("TERM"))
		fmt.Fprintf(os.Stderr, "  COLORTERM: %s\n", viper.GetString("COLORTERM"))
		fmt.Fprintf(os.Stderr, "  forceColor: %v\n", config.forceColor)
		fmt.Fprintf(os.Stderr, "  explicitlyDisabled: %v\n", config.explicitlyDisabled)
	}

	var w io.Writer
	var profile colorprofile.Profile
	var renderer *lipgloss.Renderer

	if config.explicitlyDisabled {
		colorW := colorprofile.NewWriter(cmd.OutOrStdout(), os.Environ())
		colorW.Profile = colorprofile.Ascii
		w = colorW
		profile = colorprofile.Ascii
		renderer = lipgloss.NewRenderer(w)
		renderer.SetColorProfile(termenv.Ascii)
		if config.debugColors {
			fmt.Fprintf(os.Stderr, "  Mode: Explicitly Disabled\n")
			fmt.Fprintf(os.Stderr, "  Final Profile: Ascii\n")
		}
	} else if config.forceColor {
		w = cmd.OutOrStdout()
		profile = colorprofile.ANSI256
		termOut := termenv.NewOutput(w, termenv.WithProfile(termenv.ANSI256))
		termenv.SetDefaultOutput(termOut)
		renderer = lipgloss.NewRenderer(termOut, termenv.WithProfile(termenv.ANSI256))
		if config.debugColors {
			fmt.Fprintf(os.Stderr, "  Mode: Force Color (pipe-safe)\n")
			fmt.Fprintf(os.Stderr, "  Final Profile: ANSI256 (forced)\n")
			fmt.Fprintf(os.Stderr, "  Renderer: Created with termenv.Output ANSI256 as writer\n")
			fmt.Fprintf(os.Stderr, "  Renderer ColorProfile: %v\n", renderer.ColorProfile())
			fmt.Fprintf(os.Stderr, "  Global termenv DefaultOutput profile: %v\n", termenv.DefaultOutput().ColorProfile())
		}
	} else {
		colorW := colorprofile.NewWriter(cmd.OutOrStdout(), os.Environ())
		colorW.Profile = profileDetector.Profile
		w = colorW
		profile = colorW.Profile
		renderer = lipgloss.NewRenderer(w)

		switch profile {
		case colorprofile.TrueColor:
			renderer.SetColorProfile(termenv.TrueColor)
			if config.debugColors {
				fmt.Fprintf(os.Stderr, "  Renderer: Auto-detect, set to TrueColor\n")
			}
		case colorprofile.ANSI256:
			renderer.SetColorProfile(termenv.ANSI256)
			if config.debugColors {
				fmt.Fprintf(os.Stderr, "  Renderer: Auto-detect, set to ANSI256\n")
			}
		case colorprofile.ANSI:
			renderer.SetColorProfile(termenv.ANSI)
			if config.debugColors {
				fmt.Fprintf(os.Stderr, "  Renderer: Auto-detect, set to ANSI\n")
			}
		case colorprofile.Ascii:
			renderer.SetColorProfile(termenv.Ascii)
			if config.debugColors {
				fmt.Fprintf(os.Stderr, "  Renderer: Auto-detect, set to Ascii\n")
			}
		}

		if config.debugColors {
			fmt.Fprintf(os.Stderr, "  Mode: Auto-detect\n")
			fmt.Fprintf(os.Stderr, "  Final Profile: %v\n", profile)
		}
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

// createHelpStyles creates the color styles for help output.
func createHelpStyles(renderer *lipgloss.Renderer) helpStyles {
	defer perf.Track(nil, "cmd.createHelpStyles")()

	return helpStyles{
		cyan:       renderer.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#00D7FF", Dark: "#00D7FF"}),
		green:      renderer.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0CB37F", Dark: "#0CB37F"}),
		gray:       renderer.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#626262", Dark: "#626262"}),
		dimmedGray: renderer.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#4A4A4A", Dark: "#4A4A4A"}),
		lightGray:  renderer.NewStyle().Foreground(lipgloss.Color("#e7e5e4")),
		darkGray:   renderer.NewStyle().Foreground(lipgloss.Color("#57534e")),
	}
}

// printLogoAndVersion prints the Atmos logo and version information.
func printLogoAndVersion(w io.Writer, styles helpStyles) {
	defer perf.Track(nil, "cmd.printLogoAndVersion")()

	fmt.Fprintln(w)
	_ = tuiUtils.PrintStyledTextToSpecifiedOutput(w, "ATMOS")

	versionText := styles.lightGray.Render(version.Version)
	osArchText := styles.darkGray.Render(fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH))
	versionInfo := fmt.Sprintf("👽 %s %s", versionText, osArchText)
	fmt.Fprintln(w, versionInfo)
	fmt.Fprintln(w)
}

// printDescription prints the command description.
func printDescription(w io.Writer, cmd *cobra.Command, styles helpStyles) {
	defer perf.Track(nil, "cmd.printDescription")()

	if cmd.Long != "" {
		fmt.Fprintln(w, styles.gray.Render(cmd.Long))
		fmt.Fprintln(w)
	} else if cmd.Short != "" {
		fmt.Fprintln(w, styles.gray.Render(cmd.Short))
		fmt.Fprintln(w)
	}
}

// printUsageSection prints the usage section.
func printUsageSection(w io.Writer, cmd *cobra.Command, renderer *lipgloss.Renderer, styles helpStyles) {
	defer perf.Track(nil, "cmd.printUsageSection")()

	fmt.Fprintln(w, styles.cyan.Render("USAGE"))
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
func printAliases(w io.Writer, cmd *cobra.Command, styles helpStyles) {
	defer perf.Track(nil, "cmd.printAliases")()

	if len(cmd.Aliases) > 0 {
		fmt.Fprintln(w, styles.cyan.Render("ALIASES"))
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  %s\n", styles.gray.Render(cmd.NameAndAliases()))
		fmt.Fprintln(w)
	}
}

// printSubcommandAliases prints subcommand aliases.
func printSubcommandAliases(w io.Writer, cmd *cobra.Command, styles helpStyles) {
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

	fmt.Fprintln(w, styles.cyan.Render("SUBCOMMAND ALIASES"))
	fmt.Fprintln(w)
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || len(c.Aliases) == 0 {
			continue
		}
		name := styles.green.Render(fmt.Sprintf("%-15s", c.Aliases[0]))
		desc := styles.gray.Render(fmt.Sprintf("Alias of \"%s %s\" command", cmd.Name(), c.Name()))
		fmt.Fprintf(w, "      %s  %s\n", name, desc)
	}
	fmt.Fprintln(w)
}

// printExamples prints command examples.
func printExamples(w io.Writer, cmd *cobra.Command, renderer *lipgloss.Renderer, styles helpStyles) {
	defer perf.Track(nil, "cmd.printExamples")()

	if cmd.Example == "" {
		return
	}

	fmt.Fprintln(w, styles.cyan.Render("EXAMPLES"))
	fmt.Fprintln(w)

	exampleText := strings.TrimSpace(cmd.Example)
	exampleText = strings.ReplaceAll(exampleText, "```shell", "")
	exampleText = strings.ReplaceAll(exampleText, "```", "")

	termWidth := getTerminalWidth()
	rendered := markdown.RenderCodeBlock(renderer, exampleText, termWidth)

	fmt.Fprintln(w, rendered)
	fmt.Fprintln(w)
}

// printAvailableCommands prints the list of available subcommands.
func printAvailableCommands(w io.Writer, cmd *cobra.Command, styles helpStyles) {
	defer perf.Track(nil, "cmd.printAvailableCommands")()

	if !cmd.HasAvailableSubCommands() {
		return
	}

	fmt.Fprintln(w, styles.cyan.Render("AVAILABLE COMMANDS"))
	fmt.Fprintln(w)

	// Calculate maximum command name width.
	maxCmdWidth := 0
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() && c.Name() != "help" {
			continue
		}
		cmdWidth := len(c.Name())
		if c.HasAvailableSubCommands() {
			cmdWidth += len(" [command]")
		}
		if cmdWidth > maxCmdWidth {
			maxCmdWidth = cmdWidth
		}
	}

	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() && c.Name() != "help" {
			continue
		}
		cmdName := c.Name()
		cmdTypePlain := ""
		cmdTypeStyled := ""
		if c.HasAvailableSubCommands() {
			cmdTypePlain = " [command]"
			cmdTypeStyled = " " + styles.dimmedGray.Render("[command]")
		}

		padding := maxCmdWidth - len(cmdName) - len(cmdTypePlain)

		fmt.Fprint(w, "      ")
		fmt.Fprint(w, styles.green.Render(cmdName))
		fmt.Fprint(w, cmdTypeStyled)
		fmt.Fprint(w, strings.Repeat(" ", padding))
		fmt.Fprintf(w, "  %s\n", styles.gray.Render(c.Short))
	}
	fmt.Fprintln(w)
}

// printFlags prints command flags.
func printFlags(w io.Writer, cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration, styles helpStyles) {
	defer perf.Track(nil, "cmd.printFlags")()

	termWidth := getTerminalWidth()

	if cmd.HasAvailableLocalFlags() {
		fmt.Fprintln(w, styles.cyan.Render("FLAGS"))
		fmt.Fprintln(w)
		renderFlags(w, cmd.LocalFlags(), styles.green, styles.dimmedGray, styles.gray, termWidth, atmosConfig)
		fmt.Fprintln(w)
	}

	if cmd.HasAvailableInheritedFlags() {
		fmt.Fprintln(w, styles.cyan.Render("GLOBAL FLAGS"))
		fmt.Fprintln(w)
		renderFlags(w, cmd.InheritedFlags(), styles.green, styles.dimmedGray, styles.gray, termWidth, atmosConfig)
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

	// Set custom help function.
	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		printLogoAndVersion(writerConf.writer, styles)
		printDescription(writerConf.writer, c, styles)
		printUsageSection(writerConf.writer, c, writerConf.renderer, styles)
		printAliases(writerConf.writer, c, styles)
		printSubcommandAliases(writerConf.writer, c, styles)
		printExamples(writerConf.writer, c, writerConf.renderer, styles)
		printAvailableCommands(writerConf.writer, c, styles)
		printFlags(writerConf.writer, c, &atmosConfig, styles)
		printFooter(writerConf.writer, c, &atmosConfig, styles)
	})
}

// printFooter prints the help footer message.
func printFooter(w io.Writer, cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration, styles helpStyles) {
	defer perf.Track(nil, "cmd.printFooter")()

	if !cmd.HasAvailableSubCommands() {
		return
	}

	usageMsg := fmt.Sprintf("Use `%s [command] --help` for more information about a command.", cmd.CommandPath())
	mdRenderer, err := markdown.NewTerminalMarkdownRenderer(*atmosConfig)
	if err == nil {
		rendered, renderErr := mdRenderer.RenderWithoutWordWrap(usageMsg)
		if renderErr == nil {
			usageMsg = strings.TrimSpace(rendered)
		}
	}
	fmt.Fprintf(w, "\n%s\n", styles.gray.Render(usageMsg))
}
