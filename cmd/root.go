package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"strings"
	"syscall"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/lipgloss"
	xterm "github.com/charmbracelet/x/term"
	"github.com/muesli/reflow/wordwrap"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/profiler"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui/heatmap"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
)

const (
	// LogFileMode is the file mode for log files.
	logFileMode = 0o644
	// DefaultTopFunctionsMax is the default number of top functions to display in performance summary.
	defaultTopFunctionsMax = 50
)

// atmosConfig This is initialized before everything in the Execute function. So we can directly use this.
var atmosConfig schema.AtmosConfiguration

// profilerServer holds the global profiler server instance.
var profilerServer *profiler.Server

// logFileHandle holds the opened log file for the lifetime of the program.
var logFileHandle *os.File

// RootCmd represents the base command when called without any subcommands.
var RootCmd = &cobra.Command{
	Use:                "atmos",
	Short:              "Universal Tool for DevOps and Cloud Automation",
	Long:               `Atmos is a universal tool for DevOps and cloud automation used for provisioning, managing and orchestrating workflows across various toolchains`,
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Determine if the command is a help command or if the help flag is set.
		isHelpCommand := cmd.Name() == "help"
		helpFlag := cmd.Flags().Changed("help")

		isHelpRequested := isHelpCommand || helpFlag

		if isHelpRequested {
			// Do not silence usage or errors when help is invoked.
			cmd.SilenceUsage = false
			cmd.SilenceErrors = false
		} else {
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
		}
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		// Honor CLI overrides for resolving atmos.yaml and its imports.
		if bp, _ := cmd.Flags().GetString("base-path"); bp != "" {
			configAndStacksInfo.AtmosBasePath = bp
		}
		if cfgFiles, _ := cmd.Flags().GetStringSlice("config"); len(cfgFiles) > 0 {
			configAndStacksInfo.AtmosConfigFilesFromArg = cfgFiles
		}
		if cfgDirs, _ := cmd.Flags().GetStringSlice("config-path"); len(cfgDirs) > 0 {
			configAndStacksInfo.AtmosConfigDirsFromArg = cfgDirs
		}
		// Load the config (includes env var bindings); don't store globally yet.
		tmpConfig, err := cfg.InitCliConfig(configAndStacksInfo, false)
		if err != nil {
			if errors.Is(err, cfg.NotFound) {
				// For help commands or when help flag is set, we don't want to show the error.
				if !isHelpRequested {
					log.Warn(err.Error())
				}
			} else {
				errUtils.CheckErrorPrintAndExit(err, "", "")
			}
		}

		// Setup profiler before command execution (but skip for help commands).
		if !isHelpRequested {
			if setupErr := setupProfiler(cmd, &tmpConfig); setupErr != nil {
				errUtils.CheckErrorPrintAndExit(setupErr, "Failed to setup profiler", "")
			}
		}

		// Check for --version flag (uses same code path as version command).
		if cmd.Flags().Changed("version") {
			if versionFlag, err := cmd.Flags().GetBool("version"); err == nil && versionFlag {
				versionErr := e.NewVersionExec(&tmpConfig).Execute(false, "")
				if versionErr != nil {
					errUtils.CheckErrorPrintAndExit(versionErr, "", "")
				}
				utils.OsExit(0)
				return
			}
		}

		// Enable performance tracking if heatmap flag is set.
		// P95 latency tracking via HDR histogram is automatically enabled.
		if showHeatmap, _ := cmd.Flags().GetBool("heatmap"); showHeatmap {
			perf.EnableTracking(true)
		}
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		// Stop profiler after command execution.
		if profilerServer != nil {
			if stopErr := profilerServer.Stop(); stopErr != nil {
				log.Error("Failed to stop profiler", "error", stopErr)
			}
		}

		// Show performance heatmap if enabled.
		showHeatmap, _ := cmd.Flags().GetBool("heatmap")
		if showHeatmap {
			heatmapMode, _ := cmd.Flags().GetString("heatmap-mode")
			if err := displayPerformanceHeatmap(cmd, heatmapMode); err != nil {
				log.Error("Failed to display performance heatmap", "error", err)
			}
		}
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration.
		checkAtmosConfig()

		err := e.ExecuteAtmosCmd()
		return err
	},
}

// setupLogger configures the global logger based on the provided Atmos configuration.
func setupLogger(atmosConfig *schema.AtmosConfiguration) {
	switch atmosConfig.Logs.Level {
	case "Trace":
		log.SetLevel(log.TraceLevel)
	case "Debug":
		log.SetLevel(log.DebugLevel)
	case "Info":
		log.SetLevel(log.InfoLevel)
	case "Warning":
		log.SetLevel(log.WarnLevel)
	case "Off":
		log.SetLevel(math.MaxInt32)
	default:
		log.SetLevel(log.WarnLevel)
	}

	// Always set up styles to ensure trace level shows as "TRCE".
	styles := log.DefaultStyles()

	// Set trace level to show "TRCE" instead of being blank/DEBU.
	if debugStyle, ok := styles.Levels[log.DebugLevel]; ok {
		// Copy debug style but set the string to "TRCE"
		styles.Levels[log.TraceLevel] = debugStyle.SetString("TRCE")
	} else {
		// Fallback if debug style doesn't exist.
		styles.Levels[log.TraceLevel] = lipgloss.NewStyle().SetString("TRCE")
	}

	// If colors are disabled, clear the colors but keep the level strings.
	if !atmosConfig.Settings.Terminal.IsColorEnabled() {
		clearedStyles := &log.Styles{}
		clearedStyles.Levels = make(map[log.Level]lipgloss.Style)
		for k := range styles.Levels {
			if k == log.TraceLevel {
				// Keep TRCE string but remove color
				clearedStyles.Levels[k] = lipgloss.NewStyle().SetString("TRCE")
			} else {
				// For other levels, keep their default strings but remove color
				clearedStyles.Levels[k] = styles.Levels[k].UnsetForeground().Bold(false)
			}
		}
		log.SetStyles(clearedStyles)
	} else {
		log.SetStyles(styles)
	}
	// Only set output if a log file is configured.
	if atmosConfig.Logs.File != "" {
		var output io.Writer

		switch atmosConfig.Logs.File {
		case "/dev/stderr":
			output = os.Stderr
		case "/dev/stdout":
			output = os.Stdout
		case "/dev/null":
			output = io.Discard // More efficient than opening os.DevNull
		default:
			logFile, err := os.OpenFile(atmosConfig.Logs.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, logFileMode)
			errUtils.CheckErrorPrintAndExit(err, "Failed to open log file", "")
			// Store the file handle for later cleanup instead of deferring close.
			logFileHandle = logFile
			output = logFile
		}

		log.SetOutput(output)
	}
	if _, err := log.ParseLogLevel(atmosConfig.Logs.Level); err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}
	log.Debug("Set", "logs-level", log.GetLevelString(), "logs-file", atmosConfig.Logs.File)
}

// setupColorProfile configures the global lipgloss color profile based on Atmos configuration.
func setupColorProfile(atmosConfig *schema.AtmosConfiguration) {
	// Force TrueColor profile when ATMOS_FORCE_COLOR is enabled.
	// This bypasses terminal detection and always outputs ANSI color codes.
	if atmosConfig.Settings.Terminal.ForceColor {
		lipgloss.SetColorProfile(termenv.TrueColor)
		log.SetColorProfile(termenv.TrueColor)
		log.Debug("Forced TrueColor profile", "force_color", true)
	}
}

// setupColorProfileFromEnv checks ATMOS_FORCE_COLOR environment variable early.
// This is called during init() before Boa styles are created, ensuring Cobra help
// text rendering respects the forced color profile.
func setupColorProfileFromEnv() {
	// Use viper to check environment variable to satisfy linter requirements.
	v := viper.New()
	v.SetEnvPrefix("ATMOS")
	v.AutomaticEnv()
	if v.GetBool("FORCE_COLOR") {
		lipgloss.SetColorProfile(termenv.TrueColor)
	}
}

// cleanupLogFile closes the log file handle if it was opened.
func cleanupLogFile() {
	if logFileHandle != nil {
		// Flush any remaining log data before closing.
		_ = logFileHandle.Sync()
		_ = logFileHandle.Close()
		logFileHandle = nil
	}
}

// Cleanup performs cleanup operations before the program exits.
// This should be called by main when the program is terminating.
func Cleanup() {
	cleanupLogFile()
}

// renderFlags renders a flag set with colors and proper text wrapping.
// Flag names are colored green, flag types are dimmed, descriptions are wrapped to terminal width.
// Markdown in descriptions is rendered to terminal output.
func renderFlags(w io.Writer, flags *pflag.FlagSet, flagStyle, argTypeStyle, descStyle lipgloss.Style, termWidth int, atmosConfig *schema.AtmosConfiguration) {
	if flags == nil {
		return
	}

	const (
		leftPad      = 2  // Left padding before flag names (matches commands)
		spaceBetween = 2  // Space between flag name and description
		rightMargin  = 2  // Right margin to prevent text touching edge
		minDescWidth = 40 // Minimum width for description column
	)

	// Calculate maximum flag name width
	maxFlagWidth := 0
	flags.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		flagName := formatFlagName(f)
		if len(flagName) > maxFlagWidth {
			maxFlagWidth = len(flagName)
		}
	})

	// Calculate description column width
	descColStart := leftPad + maxFlagWidth + spaceBetween
	descWidth := termWidth - descColStart - rightMargin
	if descWidth < minDescWidth {
		descWidth = minDescWidth // Ensure minimum readable width
	}

	// Create markdown renderer for flag descriptions
	renderer, err := markdown.NewTerminalMarkdownRenderer(*atmosConfig)
	if err != nil {
		renderer = nil // Fall back to plain text
	}

	// Render each flag
	flags.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}

		// Get flag name parts (without type) and the plain text length
		flagNamePlain, flagTypePlain := formatFlagNameParts(f)
		fullPlainLength := len(flagNamePlain)
		if flagTypePlain != "" {
			fullPlainLength += 1 + len(flagTypePlain) // +1 for space between name and type
		}
		padding := maxFlagWidth - fullPlainLength

		// Render flag name with colors (name in green, type in dimmed gray)
		fmt.Fprint(w, strings.Repeat(" ", leftPad))
		fmt.Fprint(w, flagStyle.Render(flagNamePlain))
		if flagTypePlain != "" {
			fmt.Fprint(w, " ")
			fmt.Fprint(w, argTypeStyle.Render(flagTypePlain))
		}
		fmt.Fprint(w, strings.Repeat(" ", padding+spaceBetween))

		// Build description with default value
		usage := f.Usage
		if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" && f.DefValue != "[]" && f.Name != "" && f.Name != "help" {
			usage += fmt.Sprintf(" (default `%s`)", f.DefValue)
		}

		// Word wrap the description first
		wrapped := wordwrap.String(usage, descWidth)

		// Render markdown if available
		if renderer != nil {
			rendered, renderErr := renderer.RenderWithoutWordWrap(wrapped)
			if renderErr == nil {
				wrapped = strings.TrimSpace(rendered)
			}
		}

		lines := strings.Split(wrapped, "\n")

		// Print first line on the same line as flag name
		if len(lines) > 0 {
			fmt.Fprintf(w, "%s\n", descStyle.Render(lines[0]))
		}

		// Print continuation lines with proper indentation
		indent := strings.Repeat(" ", descColStart)
		for i := 1; i < len(lines); i++ {
			fmt.Fprintf(w, "%s%s\n", indent, descStyle.Render(lines[i]))
		}

		// Add blank line after each flag for readability
		fmt.Fprintln(w)
	})
}

// formatFlagNameParts returns the flag name and type as separate strings for independent styling.
// Returns (flagName, flagType) where flagType may be empty for bool flags.
// Aligns all long flags (--name) at the same column regardless of shorthand presence.
func formatFlagNameParts(f *pflag.Flag) (string, string) {
	// Get the flag type (e.g., "string", "int", "bool")
	flagType := f.Value.Type()

	// Build flag name (without type)
	// Align long flags at column: "  " (leftPad=2) + "-X, " (4 chars for shorthand)
	// So long-only flags need 4 spaces before "--" to align with shorthand flags
	var flagName string
	if f.Shorthand == "" {
		flagName = fmt.Sprintf("    --%s", f.Name) // 4 spaces to align with "-X, --"
	} else {
		flagName = fmt.Sprintf("-%s, --%s", f.Shorthand, f.Name) // "-X, --name"
	}

	// For bool flags, don't return a type
	if flagType == "bool" {
		return flagName, ""
	}

	// Replace "stringSlice" with "strings" for better readability
	if flagType == "stringSlice" {
		flagType = "strings"
	}

	return flagName, flagType
}

// formatFlagName formats a flag with its shorthand (if any) and type in Cobra style.
// This is used for calculating maximum width.
func formatFlagName(f *pflag.Flag) string {
	flagName, flagType := formatFlagNameParts(f)
	if flagType != "" {
		return flagName + " " + flagType
	}
	return flagName
}

// getTerminalWidth returns the current terminal width, with a fallback default.
func getTerminalWidth() int {
	const defaultWidth = 120 // Fang's max width
	width, _, err := xterm.GetSize(os.Stdout.Fd())
	if err != nil || width <= 0 {
		return defaultWidth
	}
	if width > defaultWidth {
		return defaultWidth // Cap at maximum for readability
	}
	return width
}

// stripBackgroundCodes removes background ANSI color codes (ESC[48;...) while preserving foreground colors.
// This allows markdown-rendered content to be displayed on our custom background without conflicts.
func stripBackgroundCodes(s string) string {
	// Split by ESC character and process each escape sequence
	parts := strings.Split(s, "\x1b[")
	if len(parts) == 0 {
		return s
	}

	// First part has no escape sequence
	result := parts[0]

	// Process remaining parts (each starts with an ANSI code)
	for i := 1; i < len(parts); i++ {
		part := parts[i]
		// Find where the ANSI code ends (at a letter)
		endIdx := -1
		for j := 0; j < len(part); j++ {
			if (part[j] >= 'A' && part[j] <= 'Z') || (part[j] >= 'a' && part[j] <= 'z') {
				endIdx = j
				break
			}
		}

		if endIdx == -1 {
			// No ending found, keep as-is
			result += "\x1b[" + part
			continue
		}

		ansiCode := part[:endIdx+1]
		remainder := part[endIdx+1:]

		// Check if this is a background code (48;...)
		if !strings.HasPrefix(ansiCode, "48;") && !strings.Contains(ansiCode, ";48;") {
			// Not a background code, keep it
			result += "\x1b[" + ansiCode
		}

		result += remainder
	}

	return result
}

// applyColoredHelpTemplate applies a custom colored help template using colorprofile.Writer and lipgloss.
// This approach ensures colors work in both interactive terminals and redirected output (screengrabs).
// Colors are automatically enabled when ATMOS_FORCE_COLOR, CLICOLOR_FORCE, or FORCE_COLOR is set.
func applyColoredHelpTemplate(cmd *cobra.Command) {
	// Check if FORCE_COLOR is set (any variant) and is truthy.
	// Truthy values: "1", "true" (case-insensitive) - standard Go bool values
	// Falsy values: "0", "false" (case-insensitive) - standard Go bool values
	// Empty string means not set (use default behavior)
	isTruthy := func(val string) bool {
		if val == "" {
			return false
		}
		v := strings.ToLower(strings.TrimSpace(val))
		return v == "1" || v == "true"
	}

	isFalsy := func(val string) bool {
		if val == "" {
			return false
		}
		v := strings.ToLower(strings.TrimSpace(val))
		return v == "0" || v == "false"
	}

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

	// Determine final forceColor value:
	// - If any variant is explicitly false, disable colors
	// - Otherwise, if any variant is truthy, enable colors
	// - Otherwise, use default behavior (auto-detect TTY)
	forceColor := false
	explicitlyDisabled := isFalsy(atmosForceColor) || isFalsy(cliColorForce) || isFalsy(forceColorEnv)

	if !explicitlyDisabled {
		forceColor = isTruthy(atmosForceColor) || isTruthy(cliColorForce) || isTruthy(forceColorEnv)
	}

	// Ensure standard env vars are set for ALL color libraries:
	// - FORCE_COLOR: checked by supportscolor, figurine
	// - CLICOLOR_FORCE: checked by lipgloss/termenv
	// - NO_COLOR: universal standard for disabling colors
	if explicitlyDisabled {
		// Set NO_COLOR to disable colors in all libraries
		os.Setenv("NO_COLOR", "1")
		os.Setenv("FORCE_COLOR", "0")
		os.Setenv("CLICOLOR_FORCE", "0")
	} else if forceColor {
		// Remove NO_COLOR if present, then set force color vars.
		os.Unsetenv("NO_COLOR")
		if viper.GetString("FORCE_COLOR") == "" {
			os.Setenv("FORCE_COLOR", "1")
		}
		if viper.GetString("CLICOLOR_FORCE") == "" {
			os.Setenv("CLICOLOR_FORCE", "1")
		}
	}

	// Detect color profile from os.Stdout (for accurate TTY detection).
	// cmd.OutOrStdout() may be wrapped/buffered by Cobra, breaking TTY detection.
	profileDetector := colorprofile.NewWriter(os.Stdout, os.Environ())

	// Debug: Log detected profile information.
	debugColors := viper.GetString("ATMOS_DEBUG_COLORS") != ""
	if debugColors {
		fmt.Fprintf(os.Stderr, "\n[DEBUG] Color Detection:\n")
		fmt.Fprintf(os.Stderr, "  Detected Profile: %v\n", profileDetector.Profile)
		fmt.Fprintf(os.Stderr, "  ATMOS_FORCE_COLOR: %s\n", viper.GetString("ATMOS_FORCE_COLOR"))
		fmt.Fprintf(os.Stderr, "  FORCE_COLOR: %s\n", viper.GetString("FORCE_COLOR"))
		fmt.Fprintf(os.Stderr, "  CLICOLOR_FORCE: %s\n", viper.GetString("CLICOLOR_FORCE"))
		fmt.Fprintf(os.Stderr, "  NO_COLOR: %s\n", viper.GetString("NO_COLOR"))
		fmt.Fprintf(os.Stderr, "  TERM: %s\n", viper.GetString("TERM"))
		fmt.Fprintf(os.Stderr, "  COLORTERM: %s\n", viper.GetString("COLORTERM"))
		fmt.Fprintf(os.Stderr, "  forceColor: %v\n", forceColor)
		fmt.Fprintf(os.Stderr, "  explicitlyDisabled: %v\n", explicitlyDisabled)
	}

	var w io.Writer
	var profile colorprofile.Profile

	if explicitlyDisabled {
		// When colors are explicitly disabled, use colorprofile.Writer with Ascii profile
		// to strip all colors
		colorW := colorprofile.NewWriter(cmd.OutOrStdout(), os.Environ())
		colorW.Profile = colorprofile.Ascii
		w = colorW
		profile = colorprofile.Ascii
		if debugColors {
			fmt.Fprintf(os.Stderr, "  Mode: Explicitly Disabled\n")
			fmt.Fprintf(os.Stderr, "  Final Profile: Ascii\n")
		}
	} else if forceColor {
		// When FORCE_COLOR is set, write directly without colorprofile.Writer
		// to ensure colors are preserved in pipes and redirects.
		// Force ANSI256 profile to match TTY behavior and avoid 16-color degradation.
		w = cmd.OutOrStdout()
		profile = colorprofile.ANSI256
		if debugColors {
			fmt.Fprintf(os.Stderr, "  Mode: Force Color (pipe-safe)\n")
			fmt.Fprintf(os.Stderr, "  Final Profile: ANSI256 (forced)\n")
		}
	} else {
		// Normal mode: use colorprofile.Writer for automatic TTY detection
		colorW := colorprofile.NewWriter(cmd.OutOrStdout(), os.Environ())
		colorW.Profile = profileDetector.Profile
		w = colorW
		profile = colorW.Profile
		if debugColors {
			fmt.Fprintf(os.Stderr, "  Mode: Auto-detect\n")
			fmt.Fprintf(os.Stderr, "  Final Profile: %v\n", profile)
		}
	}

	// Create a lipgloss renderer with the detected (or forced) color profile.
	// This ensures lipgloss styles respect the color profile.
	var renderer *lipgloss.Renderer

	if forceColor {
		// When FORCE_COLOR is set, create a termenv.Output with ANSI256 profile explicitly.
		// Pass it as both the writer AND use it to set the renderer's output.
		// This ensures all color conversions use ANSI256.
		termOut := termenv.NewOutput(w, termenv.WithProfile(termenv.ANSI256))

		// CRITICAL: Set the default output GLOBALLY so lipgloss Color() uses ANSI256
		// instead of detecting ANSI (16 colors) from the pipe.
		termenv.SetDefaultOutput(termOut)

		// Create renderer using the termenv.Output as the writer.
		// termenv.Output implements io.Writer, so this works.
		renderer = lipgloss.NewRenderer(termOut, termenv.WithProfile(termenv.ANSI256))
		if debugColors {
			fmt.Fprintf(os.Stderr, "  Renderer: Created with termenv.Output ANSI256 as writer\n")
			fmt.Fprintf(os.Stderr, "  Renderer ColorProfile: %v\n", renderer.ColorProfile())
			fmt.Fprintf(os.Stderr, "  Global termenv DefaultOutput profile: %v\n", termenv.DefaultOutput().ColorProfile())
		}
	} else {
		// Normal mode: let lipgloss auto-detect based on output
		renderer = lipgloss.NewRenderer(w)

		// Map colorprofile.Profile to termenv.Profile for lipgloss.
		// This must be done regardless of forceColor to ensure auto-detection works in TTY.
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

	if debugColors {
		fmt.Fprintf(os.Stderr, "  Renderer Color Profile: %v\n", renderer.ColorProfile())
		fmt.Fprintf(os.Stderr, "  Renderer Has Dark Background: %v\n\n", renderer.HasDarkBackground())
	}

	// Force dark background mode for consistent color rendering.
	// This ensures our dark theme colors (#2F2E36 backgrounds, etc.) render correctly
	// regardless of terminal background detection.
	renderer.SetHasDarkBackground(true)

	// Define Charmbracelet-inspired color scheme using lipgloss with adaptive colors.
	// Based on Fang's aesthetic: clear hierarchy, pleasant contrast, professional look.
	cyan := renderer.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#00D7FF", Dark: "#00D7FF"})       // Section headers (bright cyan)
	green := renderer.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0CB37F", Dark: "#0CB37F"})      // Command/flag names (Fang green)
	gray := renderer.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#626262", Dark: "#626262"})       // Descriptions (dimmed)
	dimmedGray := renderer.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#4A4A4A", Dark: "#4A4A4A"}) // Flag argument types (more dimmed)

	// Print logo and version for all help pages
	fmt.Fprintln(w)
	_ = tuiUtils.PrintStyledTextToSpecifiedOutput(w, "ATMOS")

	// Print version with alien emoji for all commands - version in light gray, OS/arch in dark gray
	lightGray := renderer.NewStyle().Foreground(lipgloss.Color("#e7e5e4"))
	darkGray := renderer.NewStyle().Foreground(lipgloss.Color("#57534e"))
	versionText := lightGray.Render(version.Version)
	osArchText := darkGray.Render(fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH))
	versionInfo := fmt.Sprintf("👽 %s %s", versionText, osArchText)
	fmt.Fprintln(w, versionInfo)
	fmt.Fprintln(w)

	// Print description
	if cmd.Long != "" {
		fmt.Fprintln(w, gray.Render(cmd.Long))
		fmt.Fprintln(w)
	} else if cmd.Short != "" {
		fmt.Fprintln(w, gray.Render(cmd.Short))
		fmt.Fprintln(w)
	}

	// Load Atmos configuration for markdown rendering
	atmosConfig, _ := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)

	// Print usage section with background box (Fang style)
	fmt.Fprintln(w, cyan.Render("USAGE"))
	fmt.Fprintln(w)
	var usageContent strings.Builder
	if cmd.Runnable() {
		fmt.Fprintf(&usageContent, "$ %s\n", cmd.UseLine())
	}
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintf(&usageContent, "$ %s", cmd.CommandPath()+" [sub-command] [flags]")
	}
	if usageContent.Len() > 0 {
		// Use Fang-style code block rendering (no markdown processor, no nested backgrounds)
		usageText := strings.TrimRight(usageContent.String(), "\n")
		termWidth := getTerminalWidth()
		rendered := markdown.RenderCodeBlock(renderer, usageText, termWidth)

		fmt.Fprintln(w, rendered)
	}
	fmt.Fprintln(w)

	// Print aliases if available
	if len(cmd.Aliases) > 0 {
		fmt.Fprintln(w, cyan.Render("ALIASES"))
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  %s\n", gray.Render(cmd.NameAndAliases()))
		fmt.Fprintln(w)
	}

	// Print subcommand aliases if any commands have aliases
	hasAliases := false
	for _, c := range cmd.Commands() {
		if c.IsAvailableCommand() && len(c.Aliases) > 0 {
			hasAliases = true
			break
		}
	}
	if hasAliases {
		fmt.Fprintln(w, cyan.Render("SUBCOMMAND ALIASES"))
		fmt.Fprintln(w)
		for _, c := range cmd.Commands() {
			if !c.IsAvailableCommand() || len(c.Aliases) == 0 {
				continue
			}
			name := green.Render(fmt.Sprintf("%-15s", c.Aliases[0]))
			desc := gray.Render(fmt.Sprintf("Alias of \"%s %s\" command", cmd.Name(), c.Name()))
			fmt.Fprintf(w, "      %s  %s\n", name, desc)
		}
		fmt.Fprintln(w)
	}

	// Print examples if available with background box (Fang style)
	if cmd.Example != "" {
		fmt.Fprintln(w, cyan.Render("EXAMPLES"))
		fmt.Fprintln(w)

		// Strip markdown code fences from examples
		exampleText := strings.TrimSpace(cmd.Example)
		exampleText = strings.ReplaceAll(exampleText, "```shell", "")
		exampleText = strings.ReplaceAll(exampleText, "```", "")

		// Use Fang-style code block rendering (no markdown processor, no nested backgrounds)
		termWidth := getTerminalWidth()
		rendered := markdown.RenderCodeBlock(renderer, exampleText, termWidth)

		fmt.Fprintln(w, rendered)
		fmt.Fprintln(w)
	}

	// Print available commands with colors
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintln(w, cyan.Render("AVAILABLE COMMANDS"))
		fmt.Fprintln(w)

		// Calculate maximum command name width (including type indicator)
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
			// Command name in green, [command] in dimmed gray, description in gray
			cmdName := c.Name()
			cmdTypePlain := ""
			cmdTypeStyled := ""
			if c.HasAvailableSubCommands() {
				cmdTypePlain = " [command]"
				cmdTypeStyled = " " + dimmedGray.Render("[command]")
			}

			// Calculate padding to align descriptions
			padding := maxCmdWidth - len(cmdName) - len(cmdTypePlain)

			// Render with proper alignment (6 space indent to match flags)
			fmt.Fprint(w, "      ") // 6 spaces to align with long flags
			fmt.Fprint(w, green.Render(cmdName))
			fmt.Fprint(w, cmdTypeStyled)
			fmt.Fprint(w, strings.Repeat(" ", padding))
			fmt.Fprintf(w, "  %s\n", gray.Render(c.Short))
		}
		fmt.Fprintln(w)
	}

	// Get terminal width for proper text wrapping
	termWidth := getTerminalWidth()

	// Print flags with colors and wrapping
	if cmd.HasAvailableLocalFlags() {
		fmt.Fprintln(w, cyan.Render("FLAGS"))
		fmt.Fprintln(w)
		renderFlags(w, cmd.LocalFlags(), green, dimmedGray, gray, termWidth, &atmosConfig)
		fmt.Fprintln(w)
	}

	if cmd.HasAvailableInheritedFlags() {
		fmt.Fprintln(w, cyan.Render("GLOBAL FLAGS"))
		fmt.Fprintln(w)
		renderFlags(w, cmd.InheritedFlags(), green, dimmedGray, gray, termWidth, &atmosConfig)
		fmt.Fprintln(w)
	}

	if cmd.HasAvailableSubCommands() {
		usageMsg := fmt.Sprintf("Use `%s [command] --help` for more information about a command.", cmd.CommandPath())
		// Render markdown in footer message
		mdRenderer, err := markdown.NewTerminalMarkdownRenderer(atmosConfig)
		if err == nil {
			rendered, renderErr := mdRenderer.RenderWithoutWordWrap(usageMsg)
			if renderErr == nil {
				usageMsg = strings.TrimSpace(rendered)
			}
		}
		fmt.Fprintf(w, "\n%s\n", gray.Render(usageMsg))
	}
}

// setupProfiler initializes and starts the profiler if enabled.
func setupProfiler(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration) error {
	// Build profiler configuration from multiple sources.
	profilerConfig, err := buildProfilerConfig(cmd, atmosConfig)
	if err != nil {
		return err
	}

	// Skip when not enabled (server) and no file-based profiling requested.
	if !profilerConfig.Enabled && profilerConfig.File == "" {
		return nil
	}

	// Create and start the profiler.
	profilerServer = profiler.New(profilerConfig)
	if err := profilerServer.Start(); err != nil {
		return fmt.Errorf("%w: failed to start profiler: %v", errUtils.ErrProfilerStart, err)
	}

	return nil
}

// buildProfilerConfig constructs profiler configuration from environment and CLI flags.
func buildProfilerConfig(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration) (profiler.Config, error) {
	// Start with environment/config values or defaults.
	profilerConfig := getBaseProfilerConfig(atmosConfig)

	// Apply environment variable overrides.
	if err := applyProfilerEnvironmentOverrides(&profilerConfig, atmosConfig); err != nil {
		return profilerConfig, err
	}

	// Apply CLI flag overrides.
	if err := applyCLIFlagOverrides(&profilerConfig, cmd); err != nil {
		return profilerConfig, err
	}

	return profilerConfig, nil
}

// getBaseProfilerConfig returns the base configuration from the config file or defaults.
func getBaseProfilerConfig(atmosConfig *schema.AtmosConfiguration) profiler.Config {
	profilerConfig := atmosConfig.Profiler

	// Check if the profiler config is completely empty.
	isEmpty := profilerConfig.Host == "" &&
		profilerConfig.Port == 0 &&
		profilerConfig.File == "" &&
		profilerConfig.ProfileType == "" &&
		!profilerConfig.Enabled

	if isEmpty {
		return profiler.DefaultConfig()
	}

	// Default individual fields independently to avoid partial configurations.
	defaultConfig := profiler.DefaultConfig()
	if profilerConfig.Host == "" {
		profilerConfig.Host = defaultConfig.Host
	}
	if profilerConfig.Port == 0 {
		profilerConfig.Port = defaultConfig.Port
	}
	if profilerConfig.ProfileType == "" {
		profilerConfig.ProfileType = defaultConfig.ProfileType
	}

	return profilerConfig
}

// applyProfilerEnvironmentOverrides applies environment variable values to profiler config.
func applyProfilerEnvironmentOverrides(config *profiler.Config, atmosConfig *schema.AtmosConfiguration) error {
	if atmosConfig.Profiler.Host != "" {
		config.Host = atmosConfig.Profiler.Host
	}
	if atmosConfig.Profiler.Port != 0 {
		config.Port = atmosConfig.Profiler.Port
	}
	if atmosConfig.Profiler.File != "" {
		config.File = atmosConfig.Profiler.File
		// Enable profiler automatically when a file is specified via ENV var.
		config.Enabled = true
	}
	if atmosConfig.Profiler.ProfileType != "" {
		parsedType, parseErr := profiler.ParseProfileType(string(atmosConfig.Profiler.ProfileType))
		if parseErr != nil {
			return fmt.Errorf("%w: invalid ATMOS_PROFILE_TYPE %q: %v", errUtils.ErrParseFlag, atmosConfig.Profiler.ProfileType, parseErr)
		}
		config.ProfileType = parsedType
	}
	if atmosConfig.Profiler.Enabled {
		config.Enabled = atmosConfig.Profiler.Enabled
	}

	return nil
}

// applyCLIFlagOverrides applies CLI flag values to profiler config.
func applyCLIFlagOverrides(config *profiler.Config, cmd *cobra.Command) error {
	applyBoolFlag(cmd, "profiler-enabled", func(val bool) { config.Enabled = val })
	applyIntFlag(cmd, "profiler-port", func(val int) { config.Port = val })
	applyStringFlag(cmd, "profiler-host", func(val string) { config.Host = val })

	if err := applyProfileFileFlag(config, cmd); err != nil {
		return err
	}

	if err := applyProfileTypeFlag(config, cmd); err != nil {
		return err
	}

	return nil
}

// applyBoolFlag applies a boolean CLI flag to the config.
func applyBoolFlag(cmd *cobra.Command, flagName string, setter func(bool)) {
	if cmd.Flags().Changed(flagName) {
		if val, err := cmd.Flags().GetBool(flagName); err == nil {
			setter(val)
		}
	}
}

// applyIntFlag applies an integer CLI flag to the config.
func applyIntFlag(cmd *cobra.Command, flagName string, setter func(int)) {
	if cmd.Flags().Changed(flagName) {
		if val, err := cmd.Flags().GetInt(flagName); err == nil {
			setter(val)
		}
	}
}

// applyStringFlag applies a string CLI flag to the config.
func applyStringFlag(cmd *cobra.Command, flagName string, setter func(string)) {
	if cmd.Flags().Changed(flagName) {
		if val, err := cmd.Flags().GetString(flagName); err == nil {
			setter(val)
		}
	}
}

// applyProfileFileFlag applies the profile-file flag with auto-enable logic.
func applyProfileFileFlag(config *profiler.Config, cmd *cobra.Command) error {
	if cmd.Flags().Changed("profile-file") {
		file, err := cmd.Flags().GetString("profile-file")
		if err == nil {
			config.File = file
			// Enable profiler automatically when file is specified
			if file != "" {
				config.Enabled = true
			}
		}
	}
	return nil
}

// applyProfileTypeFlag applies the profile-type flag with validation.
func applyProfileTypeFlag(config *profiler.Config, cmd *cobra.Command) error {
	if !cmd.Flags().Changed("profile-type") {
		return nil
	}

	profileType, err := cmd.Flags().GetString("profile-type")
	if err != nil {
		return fmt.Errorf("%w: failed to get profile-type flag: %v", errUtils.ErrInvalidFlag, err)
	}

	parsedType, parseErr := profiler.ParseProfileType(profileType)
	if parseErr != nil {
		return fmt.Errorf("%w: invalid profile type '%s': %v", errUtils.ErrParseFlag, profileType, parseErr)
	}

	config.ProfileType = parsedType
	return nil
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the RootCmd.
func Execute() error {
	// InitCliConfig finds and merges CLI configurations in the following order:
	// system dir, home dir, current dir, ENV vars, command-line arguments
	// Here we need the custom commands from the config.
	var initErr error
	atmosConfig, initErr = cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)

	utils.InitializeMarkdown(atmosConfig)
	errUtils.InitializeMarkdown(atmosConfig)

	if initErr != nil && !errors.Is(initErr, cfg.NotFound) {
		if !isVersionCommand() {
			return initErr
		}
		log.Debug("Warning: CLI configuration 'atmos.yaml' file not found", "error", initErr)
	}

	// Set the log level for the charmbracelet/log package based on the atmosConfig.
	setupLogger(&atmosConfig)

	// Setup color profile for lipgloss/termenv based rendering.
	setupColorProfile(&atmosConfig)

	var err error
	// If CLI configuration was found, process its custom commands and command aliases.
	if initErr == nil {
		err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)
		if err != nil {
			return err
		}

		err = processCommandAliases(atmosConfig, atmosConfig.CommandAliases, RootCmd, true)
		if err != nil {
			return err
		}
	}

	// Boa styling is already applied via RootCmd.SetHelpFunc() which is inherited by all subcommands.
	// No need to recursively set UsageFunc as that would override Boa's handling.

	// Cobra for some reason handles root command in such a way that custom usage and help command don't work as per expectations.
	RootCmd.SilenceErrors = true
	cmd, err := RootCmd.ExecuteC()

	telemetry.CaptureCmd(cmd, err)
	if err != nil {
		if strings.Contains(err.Error(), "unknown command") {
			command := getInvalidCommandName(err.Error())
			showUsageAndExit(RootCmd, []string{command})
		}
	}
	return err
}

// getInvalidCommandName extracts the invalid command name from an error message.
func getInvalidCommandName(input string) string {
	// Regular expression to match the command name inside quotes.
	re := regexp.MustCompile(`unknown command "([^"]+)"`)

	// Find the match.
	match := re.FindStringSubmatch(input)

	// Check if a match is found.
	if len(match) > 1 {
		command := match[1] // The first capturing group contains the command
		return command
	}
	return ""
}

// displayPerformanceHeatmap shows the performance heatmap visualization.
//
//nolint:unparam // cmd parameter reserved for future use
func displayPerformanceHeatmap(cmd *cobra.Command, mode string) error {
	// Print performance summary to console.
	// Filter out functions with zero total time for cleaner output.
	snap := perf.SnapshotTopFiltered("total", defaultTopFunctionsMax)
	utils.PrintfMessageToTUI("\n=== Atmos Performance Summary ===\n")
	utils.PrintfMessageToTUI("Elapsed: %s  Functions: %d  Calls: %d\n", snap.Elapsed, snap.TotalFuncs, snap.TotalCalls)
	utils.PrintfMessageToTUI("%-50s %6s %10s %10s %10s %8s\n", "Function", "Count", "Total", "Avg", "Max", "P95")
	for _, r := range snap.Rows {
		p95 := "-"
		if r.P95 > 0 {
			p95 = heatmap.FormatDuration(r.P95)
		}
		utils.PrintfMessageToTUI("%-50s %6d %10s %10s %10s %8s\n",
			r.Name, r.Count, heatmap.FormatDuration(r.Total), heatmap.FormatDuration(r.Avg), heatmap.FormatDuration(r.Max), p95)
	}

	// Check if we have a TTY for interactive mode.
	if !term.IsTTYSupportForStderr() {
		utils.PrintfMessageToTUI("\n⚠️  No TTY available for interactive visualization. Summary displayed above.\n")
		return nil
	}

	// Create an empty heat model for now (we'll enhance this later to track actual execution steps).
	heatModel := heatmap.NewHeatModel()

	// Create context that cancels on SIGINT/SIGTERM.
	base := context.Background()
	sigCtx, stop := signal.NotifyContext(base, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start Bubble Tea UI with the collected data.
	return heatmap.StartBubbleTeaUI(sigCtx, heatModel, mode)
}

func init() {
	// Add the template function for wrapped flag usages.
	cobra.AddTemplateFunc("wrappedFlagUsages", templates.WrappedFlagUsages)

	RootCmd.PersistentFlags().String("redirect-stderr", "", "File descriptor to redirect `stderr` to. "+
		"Errors can be redirected to any file or any standard file descriptor (including `/dev/null`)")
	RootCmd.PersistentFlags().Bool("version", false, "Display the Atmos CLI version")
	RootCmd.PersistentFlags().Lookup("version").DefValue = ""

	RootCmd.PersistentFlags().String("logs-level", "Info", "Logs level. Supported log levels are Trace, Debug, Info, Warning, Off. If the log level is set to Off, Atmos will not log any messages")
	RootCmd.PersistentFlags().String("logs-file", "/dev/stderr", "The file to write Atmos logs to. Logs can be written to any file or any standard file descriptor, including '/dev/stdout', '/dev/stderr' and '/dev/null'")
	RootCmd.PersistentFlags().String("base-path", "", "Base path for Atmos project")
	RootCmd.PersistentFlags().StringSlice("config", []string{}, "Paths to configuration files (comma-separated or repeated flag)")
	RootCmd.PersistentFlags().StringSlice("config-path", []string{}, "Paths to configuration directories (comma-separated or repeated flag)")
	RootCmd.PersistentFlags().Bool("no-color", false, "Disable color output")
	RootCmd.PersistentFlags().String("pager", "", "Enable pager for output (--pager or --pager=true to enable, --pager=false to disable, --pager=less to use specific pager)")
	// Set NoOptDefVal so --pager without value means "true".
	RootCmd.PersistentFlags().Lookup("pager").NoOptDefVal = "true"
	RootCmd.PersistentFlags().Bool("profiler-enabled", false, "Enable pprof profiling server")
	RootCmd.PersistentFlags().Int("profiler-port", profiler.DefaultProfilerPort, "Port for pprof profiling server")
	RootCmd.PersistentFlags().String("profiler-host", "localhost", "Host for pprof profiling server")
	RootCmd.PersistentFlags().String("profile-file", "", "Write profiling data to file instead of starting server")
	RootCmd.PersistentFlags().String("profile-type", "cpu",
		"Type of profile to collect when using --profile-file. "+
			"Options: cpu, heap, allocs, goroutine, block, mutex, threadcreate, trace")
	RootCmd.PersistentFlags().Bool("heatmap", false, "Show performance heatmap visualization after command execution (includes P95 latency)")
	RootCmd.PersistentFlags().String("heatmap-mode", "bar", "Heatmap visualization mode: bar, sparkline, table (press 1-3 to switch in TUI)")

	// Setup color profile early for Cobra/Boa styling.
	// This must happen before initCobraConfig() creates Boa styles.
	setupColorProfileFromEnv()

	// Set custom usage template.
	err := templates.SetCustomUsageFunc(RootCmd)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	initCobraConfig()
}

// initCobraConfig initializes Cobra command configuration and styling.
func initCobraConfig() {
	RootCmd.SetOut(os.Stdout)

	RootCmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return showFlagUsageAndExit(c, err)
	})
	RootCmd.SetHelpFunc(func(command *cobra.Command, args []string) {
		contentName := strings.ReplaceAll(strings.ReplaceAll(command.CommandPath(), " ", "_"), "-", "_")
		if exampleContent, ok := examples[contentName]; ok {
			command.Example = exampleContent.Content
		}

		if !(Contains(os.Args, "help") || Contains(os.Args, "--help") || Contains(os.Args, "-h")) {
			arguments := os.Args[len(strings.Split(command.CommandPath(), " ")):]
			if len(command.Flags().Args()) > 0 {
				arguments = command.Flags().Args()
			}
			showUsageAndExit(command, arguments)
		}

		// Distinguish between interactive 'atmos help' and flag-based '--help':
		// - 'atmos help' (Contains "help" but NOT "--help" or "-h") → interactive, may use pager
		// - 'atmos --help' or 'atmos cmd --help' → simple output, NO pager unless --pager explicitly set
		isInteractiveHelp := Contains(os.Args, "help") && !(Contains(os.Args, "--help") || Contains(os.Args, "-h"))
		isFlagHelp := Contains(os.Args, "--help") || Contains(os.Args, "-h")

		// Logo and version are now printed by customRenderAtmosHelp
		telemetry.PrintTelemetryDisclosure()

		// For flag-based help (--help), render directly to stdout without buffering or pager.
		// Only use pager if --pager flag is explicitly set.
		if isFlagHelp {
			// Check if --pager flag was explicitly set.
			pagerExplicitlySet := false
			pagerEnabled := false
			if pagerFlag, err := command.Flags().GetString("pager"); err == nil && pagerFlag != "" {
				pagerExplicitlySet = true
				switch pagerFlag {
				case "true", "on", "yes", "1":
					pagerEnabled = true
				case "false", "off", "no", "0":
					pagerEnabled = false
				default:
					// Assume it's a pager command like "less" or "more"
					pagerEnabled = true
				}
			}

			if pagerExplicitlySet && pagerEnabled {
				// User explicitly requested pager for flag help.
				var buf bytes.Buffer
				command.SetOut(&buf)
				applyColoredHelpTemplate(command)
				pager := pager.NewWithAtmosConfig(true)
				_ = pager.Run("Atmos CLI Help", buf.String())
			} else {
				// Default: render help directly to stdout without pager.
				applyColoredHelpTemplate(command)
			}
		} else if isInteractiveHelp {
			// Interactive 'atmos help' command - use pager if configured.
			var buf bytes.Buffer
			command.SetOut(&buf)
			applyColoredHelpTemplate(command)

			// Check pager configuration from flag, env, or config.
			pagerEnabled := atmosConfig.Settings.Terminal.IsPagerEnabled()
			if pagerFlag, err := command.Flags().GetString("pager"); err == nil && pagerFlag != "" {
				switch pagerFlag {
				case "true", "on", "yes", "1":
					pagerEnabled = true
				case "false", "off", "no", "0":
					pagerEnabled = false
				default:
					pagerEnabled = true
				}
			}

			pager := pager.NewWithAtmosConfig(pagerEnabled)
			_ = pager.Run("Atmos CLI Help", buf.String())
		} else {
			// Fallback for other cases.
			applyColoredHelpTemplate(command)
		}

		CheckForAtmosUpdateAndPrintMessage(atmosConfig)
	})
}

// https://www.sobyte.net/post/2021-12/create-cli-app-with-cobra/
// https://github.com/spf13/cobra/blob/master/user_guide.md
// https://blog.knoldus.com/create-kubectl-like-cli-with-go-and-cobra/
// https://pkg.go.dev/github.com/c-bata/go-prompt
// https://pkg.go.dev/github.com/spf13/cobra
// https://scene-si.org/2017/04/20/managing-configuration-with-viper/
