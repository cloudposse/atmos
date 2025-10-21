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
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

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

	// Import built-in command packages for side-effect registration.
	// The init() function in each package registers the command with the registry.
	_ "github.com/cloudposse/atmos/cmd/about"
	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/cmd/version"
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

// processChdirFlag processes the --chdir flag and ATMOS_CHDIR environment variable,
// changing the working directory before any other operations.
// Precedence: --chdir flag > ATMOS_CHDIR environment variable.
func processChdirFlag(cmd *cobra.Command) error {
	chdir, _ := cmd.Flags().GetString("chdir")
	// If flag is not set, check environment variable.
	// Note: chdir is not supported in atmos.yaml since it must be processed before atmos.yaml is loaded.
	if chdir == "" {
		//nolint:forbidigo // Must use os.Getenv: chdir is processed before Viper configuration loads.
		chdir = os.Getenv("ATMOS_CHDIR")
	}

	if chdir == "" {
		return nil // No chdir specified.
	}

	// Clean and make absolute to handle both relative and absolute paths.
	absPath, err := filepath.Abs(chdir)
	if err != nil {
		return fmt.Errorf("%w: invalid chdir path: %s", errUtils.ErrPathResolution, chdir)
	}

	// Verify the directory exists before attempting to change to it.
	stat, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: directory does not exist: %s", errUtils.ErrWorkdirNotExist, absPath)
		}
		return fmt.Errorf("%w: failed to access directory: %s", errUtils.ErrStatFile, absPath)
	}

	if !stat.IsDir() {
		return fmt.Errorf("%w: not a directory: %s", errUtils.ErrWorkdirNotExist, absPath)
	}

	// Change to the specified directory.
	if err := os.Chdir(absPath); err != nil {
		return fmt.Errorf("%w: failed to change directory to %s", errUtils.ErrPathResolution, absPath)
	}

	return nil
}

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

		// Process --chdir flag before any other operations (including config loading).
		if err := processChdirFlag(cmd); err != nil {
			errUtils.CheckErrorPrintAndExit(err, "", "")
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
				errUtils.OsExit(0)
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
	defer perf.Track(atmosConfig, "root.setupColorProfile")()

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

// RenderFlags renders a flag set with colors and proper text wrapping.
// Flag names are colored green, flag types are dimmed, descriptions are wrapped to terminal width.
// Markdown in descriptions is rendered to terminal output.

// flagRenderLayout holds layout constants and dimensions for flag rendering.
type flagRenderLayout struct {
	leftPad      int
	spaceBetween int
	rightMargin  int
	minDescWidth int
	maxFlagWidth int
	descColStart int
	descWidth    int
}

// newFlagRenderLayout creates a new layout configuration for flag rendering.
func newFlagRenderLayout(termWidth int, maxFlagWidth int) flagRenderLayout {
	const (
		leftPad      = 2
		spaceBetween = 2
		rightMargin  = 2
		minDescWidth = 40
	)

	descColStart := leftPad + maxFlagWidth + spaceBetween
	descWidth := termWidth - descColStart - rightMargin
	if descWidth < minDescWidth {
		descWidth = minDescWidth
	}

	return flagRenderLayout{
		leftPad:      leftPad,
		spaceBetween: spaceBetween,
		rightMargin:  rightMargin,
		minDescWidth: minDescWidth,
		maxFlagWidth: maxFlagWidth,
		descColStart: descColStart,
		descWidth:    descWidth,
	}
}

// calculateMaxFlagWidth finds the maximum flag name width for alignment.
func calculateMaxFlagWidth(flags *pflag.FlagSet) int {
	maxWidth := 0
	flags.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		flagName := formatFlagName(f)
		if len(flagName) > maxWidth {
			maxWidth = len(flagName)
		}
	})
	return maxWidth
}

// buildFlagDescription creates the flag description with default value if applicable.
func buildFlagDescription(f *pflag.Flag) string {
	usage := f.Usage
	if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" && f.DefValue != "[]" && f.Name != "" && f.Name != "help" {
		usage += fmt.Sprintf(" (default `%s`)", f.DefValue)
	}
	return usage
}

// renderWrappedLines renders wrapped description lines with proper indentation.
func renderWrappedLines(w io.Writer, lines []string, indent int, descStyle *lipgloss.Style) {
	if len(lines) == 0 {
		return
	}

	// Print first line (already positioned on same line as flag name)
	fmt.Fprintf(w, "%s\n", descStyle.Render(lines[0]))

	// Print continuation lines with proper indentation
	indentStr := strings.Repeat(" ", indent)
	for i := 1; i < len(lines); i++ {
		fmt.Fprintf(w, "%s%s\n", indentStr, descStyle.Render(lines[i]))
	}
}

// flagStyles holds the lipgloss styles for flag rendering.
type flagStyles struct {
	flagStyle    lipgloss.Style
	argTypeStyle lipgloss.Style
	descStyle    lipgloss.Style
}

// renderSingleFlag renders one flag with its description.
func renderSingleFlag(w io.Writer, f *pflag.Flag, layout flagRenderLayout, styles *flagStyles, renderer *markdown.Renderer) {
	// Get flag name parts and calculate padding
	flagNamePlain, flagTypePlain := formatFlagNameParts(f)
	fullPlainLength := len(flagNamePlain)
	if flagTypePlain != "" {
		fullPlainLength += 1 + len(flagTypePlain)
	}
	padding := layout.maxFlagWidth - fullPlainLength

	const space = " "

	// Render flag name with colors
	fmt.Fprint(w, strings.Repeat(space, layout.leftPad))
	fmt.Fprint(w, styles.flagStyle.Render(flagNamePlain))
	if flagTypePlain != "" {
		fmt.Fprint(w, space)
		fmt.Fprint(w, styles.argTypeStyle.Render(flagTypePlain))
	}
	fmt.Fprint(w, strings.Repeat(space, padding+layout.spaceBetween))

	// Build and process description
	usage := buildFlagDescription(f)
	wrapped := wordwrap.String(usage, layout.descWidth)

	if renderer != nil {
		rendered, err := renderer.RenderWithoutWordWrap(wrapped)
		if err == nil {
			wrapped = strings.TrimSpace(rendered)
		}
	}

	lines := strings.Split(wrapped, "\n")
	renderWrappedLines(w, lines, layout.descColStart, &styles.descStyle)

	fmt.Fprintln(w)
}

// renderFlags renders all flags with formatting and styling.
//
//nolint:revive,gocritic // Function signature required for compatibility with help template system.
func renderFlags(w io.Writer, flags *pflag.FlagSet, flagStyle, argTypeStyle, descStyle lipgloss.Style, termWidth int, atmosConfig *schema.AtmosConfiguration) {
	if flags == nil {
		return
	}

	maxFlagWidth := calculateMaxFlagWidth(flags)
	layout := newFlagRenderLayout(termWidth, maxFlagWidth)

	styles := &flagStyles{
		flagStyle:    flagStyle,
		argTypeStyle: argTypeStyle,
		descStyle:    descStyle,
	}

	renderer, err := markdown.NewTerminalMarkdownRenderer(*atmosConfig)
	if err != nil {
		renderer = nil
	}

	flags.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		renderSingleFlag(w, f, layout, styles, renderer)
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

// findAnsiCodeEnd finds the index where an ANSI escape code ends (at a letter).
func findAnsiCodeEnd(s string) int {
	for i := 0; i < len(s); i++ {
		if (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z') {
			return i
		}
	}
	return -1
}

// isBackgroundCode checks if an ANSI code is a background color code (48;...).
func isBackgroundCode(ansiCode string) bool {
	return strings.HasPrefix(ansiCode, "48;") || strings.Contains(ansiCode, ";48;")
}

// processAnsiEscapeSequence processes a single ANSI escape sequence part.
func processAnsiEscapeSequence(part string) (codeToKeep string, remainder string) {
	endIdx := findAnsiCodeEnd(part)
	if endIdx == -1 {
		// No ending found, keep entire part as-is
		return "\x1b[" + part, ""
	}

	ansiCode := part[:endIdx+1]
	remainder = part[endIdx+1:]

	// Keep non-background codes
	if !isBackgroundCode(ansiCode) {
		codeToKeep = "\x1b[" + ansiCode
	}

	return codeToKeep, remainder
}

// StripBackgroundCodes removes background ANSI color codes (ESC[48;...) while preserving foreground colors.
// This allows markdown-rendered content to be displayed on our custom background without conflicts.
//
//nolint:godot // Function comment format acceptable despite linter warning.
func stripBackgroundCodes(s string) string {
	defer perf.Track(nil, "cmd.stripBackgroundCodes")()

	parts := strings.Split(s, "\x1b[")
	if len(parts) == 0 {
		return s
	}

	// First part has no escape sequence
	result := parts[0]

	// Process remaining parts (each starts with an ANSI code)
	for i := 1; i < len(parts); i++ {
		codeToKeep, remainder := processAnsiEscapeSequence(parts[i])
		result += codeToKeep + remainder
	}

	return result
}

// applyColoredHelpTemplate applies a custom colored help template using colorprofile.Writer and lipgloss.
// This approach ensures colors work in both interactive terminals and redirected output (screengrabs).
// Colors are automatically enabled when ATMOS_FORCE_COLOR, CLICOLOR_FORCE, or FORCE_COLOR is set.

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

	// Set atmosConfig for version command (needs access to config).
	version.SetAtmosConfig(&atmosConfig)

	utils.InitializeMarkdown(atmosConfig)
	errUtils.InitializeMarkdown(atmosConfig)

	if initErr != nil && !errors.Is(initErr, cfg.NotFound) {
		if !isVersionCommand() {
			return initErr
		}
		log.Debug("Warning: failed to initialize CLI configuration", "error", initErr)
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
	// Register built-in commands from the registry.
	// This must happen BEFORE custom commands are processed in Execute().
	// Commands register themselves via init() functions when their packages
	// are imported with blank imports (e.g., _ "github.com/cloudposse/atmos/cmd/about").
	if err := internal.RegisterAll(RootCmd); err != nil {
		log.Error("Failed to register built-in commands", "error", err)
	}

	// Add the template function for wrapped flag usages.
	cobra.AddTemplateFunc("wrappedFlagUsages", templates.WrappedFlagUsages)

	RootCmd.PersistentFlags().StringP("chdir", "C", "", "Change working directory before processing (run as if Atmos started in this directory)")
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

	// Bind environment variables for GitHub authentication.
	// ATMOS_GITHUB_TOKEN takes precedence over GITHUB_TOKEN.
	if err := viper.BindEnv("ATMOS_GITHUB_TOKEN", "ATMOS_GITHUB_TOKEN", "GITHUB_TOKEN"); err != nil {
		log.Error("Failed to bind ATMOS_GITHUB_TOKEN environment variable", "error", err)
	}

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
		isInteractiveHelp := Contains(os.Args, "help") && !Contains(os.Args, "--help") && !Contains(os.Args, "-h")
		isFlagHelp := Contains(os.Args, "--help") || Contains(os.Args, "-h")

		// Logo and version are now printed by customRenderAtmosHelp
		telemetry.PrintTelemetryDisclosure()

		// For flag-based help (--help), render directly to stdout without buffering or pager.
		// Only use pager if --pager flag is explicitly set.
		switch {
		case isFlagHelp:
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
					// Assume it's a pager command like "less" or "more".
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
		case isInteractiveHelp:
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
			if err := pager.Run("Atmos CLI Help", buf.String()); err != nil {
				log.Error("Failed to run pager", "error", err)
				errUtils.OsExit(1)
			}
		default:
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
