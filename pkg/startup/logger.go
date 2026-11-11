package startup

import (
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/diagnostics"
	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	terminalenv "github.com/cloudposse/atmos/pkg/terminal/env"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// LogFileMode is the file mode for log files.
	logFileMode = 0o644
	// UsageExitCode is the exit status for invalid flag or environment values.
	usageExitCode = 2

	logsLevelFlagName = "logs-level"
	logsFileFlagName  = "logs-file"
	verboseFlagName   = "verbose"
	verboseShorthand  = "v"
	// The long-flag prefix, also the bare "--" end-of-options delimiter.
	flagPrefix = "--"

	defaultLogsLevel = "Warning"
	defaultLogsFile  = "/dev/stderr"
	debugLogsLevel   = "Debug"
	logsLevelEnvVar  = "ATMOS_LOGS_LEVEL"
	logsFileEnvVar   = "ATMOS_LOGS_FILE"
	verboseEnvVar    = "ATMOS_VERBOSE"
	trueValue        = "true"
)

// GlobalColorDisabled combines configured and early command-line opt-outs before applying forced color.
func GlobalColorDisabled(config *schema.AtmosConfiguration) bool {
	defer perf.Track(config, "startup.GlobalColorDisabled")()

	// Terminal.NoColor already reflects an explicit --no-color flag (setLogConfig overrides
	// environment-derived values), and ResolveNoColor covers the startup path that runs
	// before Cobra parses flags so --no-color=false still beats ATMOS_NO_COLOR.
	return config.Settings.Terminal.NoColor ||
		terminalenv.ResolveNoColor(os.Args[1:], viper.GetBool("no-color"))
}

// LogsConfigFromCommand builds the minimal logs configuration for commands that never load atmos.yaml.
// Precedence matches configuration loading: defaults, then ATMOS_LOGS_*, then ATMOS_VERBOSE, then the
// parsed --logs-level and --logs-file flags, then --verbose.
func LogsConfigFromCommand(cmd *cobra.Command) schema.Logs {
	defer perf.Track(nil, "startup.LogsConfigFromCommand")()

	logs := schema.Logs{Level: defaultLogsLevel, File: defaultLogsFile}
	//nolint:forbidigo // Logging is configured before Viper loads any configuration.
	if level := os.Getenv(logsLevelEnvVar); level != "" {
		logs.Level = level
	}
	//nolint:forbidigo // Logging is configured before Viper loads any configuration.
	if file := os.Getenv(logsFileEnvVar); file != "" {
		logs.File = file
	}
	//nolint:forbidigo // Logging is configured before Viper loads any configuration.
	if os.Getenv(verboseEnvVar) == trueValue {
		logs.Level = debugLogsLevel
	}
	if cmd == nil {
		return logs
	}
	if cmd.Flags().Changed(logsLevelFlagName) {
		if level, err := cmd.Flags().GetString(logsLevelFlagName); err == nil {
			logs.Level = level
		}
	}
	if cmd.Flags().Changed(logsFileFlagName) {
		if file, err := cmd.Flags().GetString(logsFileFlagName); err == nil {
			logs.File = file
		}
	}
	if verbose, _ := cmd.Flags().GetBool(verboseFlagName); verbose {
		logs.Level = debugLogsLevel
	}
	return logs
}

// LogsConfigFromArgs builds the minimal logs configuration from raw CLI args and the
// ATMOS_LOGS_* / ATMOS_VERBOSE environment variables, without a parsed cobra.Command.
//
// It exists for the startup path that runs BEFORE Cobra parses flags: when configuration
// loading fails, the logger must still honor an explicitly requested level (via --logs-level,
// --verbose/-v, or ATMOS_LOGS_LEVEL/ATMOS_VERBOSE) instead of silently falling back to Warning,
// otherwise a legitimately requested Debug/Trace diagnostic is filtered out.
//
// The argv here is the whole command line, which also carries flags meant for subcommands. This
// deliberately does NOT emulate Cobra's parsing: without the full flag registry it is impossible to
// know which tokens are operands of other value-taking flags (e.g. "--format --logs-level" makes
// "--logs-level" the value of "--format"), so any attempt to consume a following token as a value
// mis-parses some argv and could, for example, open an unexpected log file. Instead it recognizes
// ONLY the unambiguous equals-joined forms (--logs-level=..., --logs-file=...), the bare boolean
// verbose flags (--verbose, -v, --verbose=<bool>), and stops at the "--" end-of-options delimiter.
// Nothing is consumed from a following token, so no reordering of unrelated flags can change the
// result. The space-separated forms ("--logs-level Debug", "--verbose false") are intentionally not
// honored here — the full command path parses those; this best-effort fallback covers the common
// equals form and the environment variables, which is enough to keep an explicitly requested
// diagnostic visible. Precedence matches LogsConfigFromCommand: defaults, then
// ATMOS_LOGS_*/ATMOS_VERBOSE, then --logs-level/--logs-file, then verbose last.
func LogsConfigFromArgs(args []string) schema.Logs {
	defer perf.Track(nil, "startup.LogsConfigFromArgs")()

	logs := LogsConfigFromCommand(nil) // Defaults + ATMOS_LOGS_*/ATMOS_VERBOSE.

	levelPrefix := flagPrefix + logsLevelFlagName + "="
	filePrefix := flagPrefix + logsFileFlagName + "="

	verbose, verboseSet := false, false
	for _, arg := range args {
		// A bare "--" ends option parsing; everything after is positional. Stop so a later
		// equals-joined logging flag is not honored (and so no unexpected log file is opened).
		if arg == flagPrefix {
			break
		}
		switch {
		case strings.HasPrefix(arg, levelPrefix):
			logs.Level = strings.TrimPrefix(arg, levelPrefix)
		case strings.HasPrefix(arg, filePrefix):
			logs.File = strings.TrimPrefix(arg, filePrefix)
		default:
			if on, matched := verboseArg(arg); matched {
				verbose, verboseSet = on, true
			}
		}
	}
	// Verbose is applied last so it wins over an explicit --logs-level, matching LogsConfigFromCommand.
	if verboseSet && verbose {
		logs.Level = debugLogsLevel
	}
	return logs
}

// verboseArg reports whether arg is a recognized verbose flag and, if so, whether it enables verbose
// logging. It matches the bare "--verbose"/"-v" (enable) and the equals-joined "--verbose=<bool>"
// forms; a separate boolean value is deliberately not consumed (see LogsConfigFromArgs).
func verboseArg(arg string) (enabled, matched bool) {
	switch {
	case arg == flagPrefix+verboseFlagName, arg == "-"+verboseShorthand:
		return true, true
	case strings.HasPrefix(arg, flagPrefix+verboseFlagName+"="):
		on, err := strconv.ParseBool(strings.TrimPrefix(arg, flagPrefix+verboseFlagName+"="))
		return err == nil && on, true
	default:
		return false, false
	}
}

// ConfigureLogger applies --logs-level, --logs-file, and their ATMOS_LOGS_* variables without reading
// atmos.yaml. The returned file handle, when not nil, is the opened log file; the caller owns closing it.
// An unsupported level is returned as an error with the usage exit code instead of exiting.
func ConfigureLogger(cmd *cobra.Command) (*os.File, error) {
	defer perf.Track(nil, "startup.ConfigureLogger")()

	config := &schema.AtmosConfiguration{Logs: LogsConfigFromCommand(cmd)}
	handle, err := SetupLogger(config)
	if err != nil {
		return handle, errUtils.WithExitCode(err, usageExitCode)
	}
	return handle, nil
}

// SetupLogger configures the global logger based on the provided Atmos configuration.
// It returns the opened log file when the configuration names a regular file, so the caller can close it
// on shutdown, and an error when the log file cannot be opened or the log level is unsupported.
func SetupLogger(atmosConfig *schema.AtmosConfiguration) (*os.File, error) {
	defer perf.Track(atmosConfig, "startup.SetupLogger")()

	disabled := GlobalColorDisabled(atmosConfig)
	log.Default().SetColorEnabled(atmosConfig.Logs.Color == nil || *atmosConfig.Logs.Color, disabled)
	forceColor := !disabled && (atmosConfig.Settings.Terminal.ForceColor || viper.GetBool("force-color"))
	if disabled {
		ui.SetColorProfile(termenv.Ascii)
	}
	if forceColor {
		lipgloss.SetColorProfile(termenv.TrueColor)
		log.SetColorProfile(termenv.TrueColor)
		theme.InvalidateStyleCache()
	}
	diagnostics.Configure(atmosConfig.Diagnostics)

	applyLogLevel(atmosConfig.Logs.Level)
	log.SetStyles(logStyles(atmosConfig, disabled, forceColor))

	// Only set output if a log file is configured.
	var handle *os.File
	if atmosConfig.Logs.File != "" {
		var output io.Writer
		var err error
		output, handle, err = logOutput(atmosConfig.Logs.File)
		if err != nil {
			return nil, err
		}
		log.SetOutput(output)
	}
	if forceColor {
		// A new log destination resets the profile, so apply forced color after SetOutput.
		log.SetColorProfile(termenv.TrueColor)
	}
	if err := validateLogLevel(atmosConfig.Logs.Level); err != nil {
		return handle, err
	}
	log.Debug("Set", "logs-level", log.GetLevelString(), "logs-file", atmosConfig.Logs.File)
	return handle, nil
}

// applyLogLevel sets the global log level; an unrecognized level falls back to Warning and is reported
// separately by validateLogLevel.
func applyLogLevel(level string) {
	switch level {
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
}

// logStyles returns the theme-aware log styles, or the no-color styles when color is off.
func logStyles(atmosConfig *schema.AtmosConfiguration, disabled, forceColor bool) *log.Styles {
	var styles *log.Styles
	if !disabled && (atmosConfig.Logs.Color == nil || *atmosConfig.Logs.Color) && (forceColor || atmosConfig.Settings.Terminal.IsColorEnabled(term.IsTTYSupportForStderr())) {
		// Get color scheme for the configured theme.
		scheme, err := theme.GetColorSchemeForTheme(atmosConfig.Settings.Terminal.Theme)
		if err == nil && scheme != nil {
			// Use themed styles.
			styles = theme.GetLogStyles(scheme)
		} else {
			// Fallback to default styles if theme loading fails.
			styles = log.DefaultStyles()
		}
	} else {
		// Use no-color styles.
		styles = theme.GetLogStylesNoColor()
	}

	// Add TRCE level for trace logging (using same style as DEBU).
	if debugStyle, ok := styles.Levels[log.DebugLevel]; ok {
		styles.Levels[log.TraceLevel] = debugStyle.SetString("TRCE")
	} else {
		styles.Levels[log.TraceLevel] = lipgloss.NewStyle().SetString("TRCE")
	}
	return styles
}

// logOutput resolves the writer for the configured log file. Only a regular file yields a handle.
func logOutput(file string) (io.Writer, *os.File, error) {
	switch file {
	case "/dev/stderr":
		return iolib.MaskWriter(os.Stderr), nil, nil
	case "/dev/stdout":
		return iolib.MaskWriter(os.Stdout), nil, nil
	case "/dev/null":
		return io.Discard, nil, nil // More efficient than opening os.DevNull.
	}
	logFile, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, logFileMode)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: log file %q: %w", errUtils.ErrOpenFile, file, err)
	}
	return iolib.MaskWriter(logFile), logFile, nil
}

// validateLogLevel reports an unsupported log level as a user-facing error.
func validateLogLevel(level string) error {
	_, err := log.ParseLogLevel(level)
	if err == nil {
		return nil
	}
	// Enrich the error with proper formatting for user-facing output.
	// The error from ParseLogLevel has format: "sentinel\nexplanation"
	// Extract the explanation text (everything after the first newline).
	_, explanation, _ := strings.Cut(err.Error(), "\n")

	// Build enriched error from the wrapped error, adding the explanation as a detail.
	// This ensures the formatter can extract both the sentinel message and the explanation.
	return errUtils.Build(log.ErrInvalidLogLevel).
		WithExplanation(explanation).
		Err()
}
