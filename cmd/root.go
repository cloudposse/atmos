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
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/elewis787/boa"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/flags"
	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/profiler"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/heatmap"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/utils"

	// Import built-in command packages for side-effect registration.
	// The init() function in each package registers the command with the registry.
	_ "github.com/cloudposse/atmos/cmd/about"
	"github.com/cloudposse/atmos/cmd/devcontainer"
	"github.com/cloudposse/atmos/cmd/internal"
	_ "github.com/cloudposse/atmos/cmd/list"
	_ "github.com/cloudposse/atmos/cmd/profile"
	themeCmd "github.com/cloudposse/atmos/cmd/theme"
	"github.com/cloudposse/atmos/cmd/version"
)

const (
	// LogFileMode is the file mode for log files.
	logFileMode = 0o644
	// DefaultTopFunctionsMax is the default number of top functions to display in performance summary.
	defaultTopFunctionsMax = 50
	// VerboseFlagName is the name of the verbose flag.
	verboseFlagName = "verbose"
)

// atmosConfig This is initialized before everything in the Execute function. So we can directly use this.
var atmosConfig schema.AtmosConfiguration

// profilerServer holds the global profiler server instance.
var profilerServer *profiler.Server

// logFileHandle holds the opened log file for the lifetime of the program.
var logFileHandle *os.File

// chdirProcessed tracks whether chdir has already been processed to avoid double-processing.
var chdirProcessed bool

// parseChdirFromArgs manually parses --chdir or -C flag from os.Args.
// This is needed for commands with DisableFlagParsing=true (terraform, helmfile, packer)
// parseChdirFromArgs scans the process arguments for a chdir flag and returns the specified path if present.
// It recognizes `--chdir=value`, `--chdir value`, `-C=value`, `-Cvalue`, and `-C value` forms.
// If no chdir flag is found, it returns an empty string.
func parseChdirFromArgs() string {
	args := os.Args
	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Check for --chdir=value format.
		if strings.HasPrefix(arg, "--chdir=") {
			return strings.TrimPrefix(arg, "--chdir=")
		}

		// Check for -C=value format.
		if strings.HasPrefix(arg, "-C=") {
			return strings.TrimPrefix(arg, "-C=")
		}

		// Check for -C<value> format (concatenated, e.g., -C../foo).
		if strings.HasPrefix(arg, "-C") && len(arg) > 2 {
			return arg[2:]
		}

		// Check for --chdir value or -C value format (next arg is the value).
		if arg == "--chdir" || arg == "-C" {
			if i+1 < len(args) {
				return args[i+1]
			}
		}
	}
	return ""
}

// processEarlyChdirFlag processes --chdir flag before RootCmd is fully initialized.
// This is called in Execute() before loading atmos.yaml to ensure the config is loaded
// processEarlyChdirFlag changes the current working directory based on the first
// occurrence of the `--chdir`/`-C` flag (parsed from os.Args) or the ATMOS_CHDIR
// environment variable, expanding `~`, resolving to an absolute path, and
// validating that the target exists and is a directory.
// It returns nil on success or an error describing why the directory could not
// be resolved or changed.
func processEarlyChdirFlag() error {
	// If chdir already processed, skip (avoid double-processing).
	if chdirProcessed {
		return nil
	}

	// Parse --chdir from os.Args since we can't use Cobra flags yet.
	chdir := parseChdirFromArgs()

	// If flag is not set, check environment variable.
	if chdir == "" {
		//nolint:forbidigo // Must use os.Getenv: chdir is processed before Viper configuration loads.
		chdir = os.Getenv("ATMOS_CHDIR")
	}

	if chdir == "" {
		return nil // No chdir specified.
	}

	// Expand tilde to home directory using filesystem package.
	homeDirProvider := filesystem.NewOSHomeDirProvider()
	expandedPath, err := homeDirProvider.Expand(chdir)
	if err != nil {
		return fmt.Errorf("%w: %s", errUtils.ErrPathResolution, err)
	}

	// Clean and make absolute to handle both relative and absolute paths.
	absPath, err := filepath.Abs(expandedPath)
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

	// Mark as processed to avoid double-processing in PersistentPreRun.
	chdirProcessed = true

	return nil
}

// syncGlobalFlagsToViper synchronizes global flags from Cobra's FlagSet to Viper.
// This is necessary because Viper's BindPFlag doesn't immediately sync values when flags are parsed.
// Call this after Cobra parses flags but before accessing flag values via Viper.
//
// Background: When using viper.BindPFlag(), the binding happens at initialization time,
// but the actual flag value isn't synced to Viper until you call viper.Get*().
// For some code paths (especially in InitCliConfig), we need the flag values
// available in Viper before config loading completes.
//
// This function explicitly syncs changed flags to Viper, making their values
// immediately available via viper.Get*() calls.
func syncGlobalFlagsToViper(cmd *cobra.Command) {
	v := viper.GetViper()

	// Sync profile flag if explicitly set.
	if cmd.Flags().Changed("profile") {
		if profiles, err := cmd.Flags().GetStringSlice("profile"); err == nil {
			v.Set("profile", profiles)
		}
	}

	// Sync identity flag if explicitly set.
	if cmd.Flags().Changed("identity") {
		if identity, err := cmd.Flags().GetString("identity"); err == nil {
			v.Set("identity", identity)
		}
	}
}

// processChdirFlag processes the --chdir flag and ATMOS_CHDIR environment variable,
// changing the working directory before any other operations.
// Precedence: --chdir flag > ATMOS_CHDIR environment variable.
// Note: This is also called from PersistentPreRun, but will be a no-op if
// processChdirFlag changes the current working directory when a chdir target is provided
// via the --chdir flag (or by parsing os.Args when flag parsing is disabled) or the
// ATMOS_CHDIR environment variable, and marks the change as processed to avoid
// double-processing.
//
// It expands a leading tilde, resolves the path to an absolute location, verifies the
// path exists and is a directory, then calls os.Chdir. If the chdir has already been
// handled, the function returns immediately. It returns an error when the path cannot
// be resolved, does not exist, is not a directory, or when changing directories fails.
func processChdirFlag(cmd *cobra.Command) error {
	// If chdir already processed in Execute(), skip to avoid double-processing.
	if chdirProcessed {
		return nil
	}

	// Try to get chdir from parsed flags first (works when DisableFlagParsing=false).
	chdir, _ := cmd.Flags().GetString("chdir")

	// If flag parsing is disabled (terraform/helmfile/packer commands), manually parse os.Args.
	// This is necessary because Cobra doesn't parse flags when DisableFlagParsing=true,
	// but PersistentPreRun runs before the command's Run function where flags would be manually parsed.
	if chdir == "" {
		chdir = parseChdirFromArgs()
	}
	// If flag is not set, check environment variable.
	// Note: chdir is not supported in atmos.yaml since it must be processed before atmos.yaml is loaded.
	if chdir == "" {
		//nolint:forbidigo // Must use os.Getenv: chdir is processed before Viper configuration loads.
		chdir = os.Getenv("ATMOS_CHDIR")
	}

	if chdir == "" {
		return nil // No chdir specified.
	}

	// Expand tilde to home directory using filesystem package.
	homeDirProvider := filesystem.NewOSHomeDirProvider()
	expandedPath, err := homeDirProvider.Expand(chdir)
	if err != nil {
		return fmt.Errorf("%w: %s", errUtils.ErrPathResolution, err)
	}

	// Clean and make absolute to handle both relative and absolute paths.
	absPath, err := filepath.Abs(expandedPath)
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

	// Mark as processed to avoid double-processing.
	chdirProcessed = true

	return nil
}

// RootCmd represents the base command when called without any subcommands.
var RootCmd = &cobra.Command{
	Use:   "atmos",
	Short: "Universal Tool for DevOps and Cloud Automation",
	Long:  `Atmos is a universal tool for DevOps and cloud automation used for provisioning, managing and orchestrating workflows across various toolchains`,
	// Note: FParseErrWhitelist is NOT set on RootCmd to allow proper flag validation.
	// Individual commands that need to pass through flags (terraform, helmfile, packer)
	// set FParseErrWhitelist{UnknownFlags: true} explicitly.
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Set verbose flag for error formatting before any command execution or fatal exits.
		if cmd.Flags().Changed(verboseFlagName) {
			// CLI flag explicitly set - use it.
			verbose, flagErr := cmd.Flags().GetBool(verboseFlagName)
			if flagErr != nil {
				errUtils.CheckErrorPrintAndExit(flagErr, "", "")
			}
			errUtils.SetVerboseFlag(verbose)
		} else if viper.IsSet(verboseFlagName) {
			// CLI flag not set - check environment variable via Viper.
			verbose := viper.GetBool(verboseFlagName)
			errUtils.SetVerboseFlag(verbose)
		}

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

		// Sync global flags from Cobra to Viper before InitCliConfig.
		// This ensures flag values are immediately available in Viper for config loading.
		syncGlobalFlagsToViper(cmd)

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
			} else if isVersionCommand() {
				// Version command should always work, even with invalid config.
				// Log config error but allow version command to proceed.
				log.Debug("CLI configuration error (continuing for version command)", "error", err)
			} else {
				// Enrich config errors with helpful context.
				enrichedErr := errUtils.Build(err).
					WithHint("Verify your atmos.yaml syntax and configuration").
					WithHint("Run 'atmos version' to check if Atmos is working").
					WithExitCode(2). // Config/usage error
					Err()
				errUtils.CheckErrorPrintAndExit(enrichedErr, "", "")
			}
		}

		// Setup profiler before command execution (but skip for help commands).
		if !isHelpRequested {
			if setupErr := setupProfiler(cmd, &tmpConfig); setupErr != nil {
				errUtils.CheckErrorPrintAndExit(setupErr, "Failed to setup profiler", "")
			}
		}

		// Check for --version flag (uses same code path as version command).
		// Use ExitCodeError{Code: 0} to signal successful completion without deep exit.
		if cmd.Flags().Changed("version") {
			if versionFlag, err := cmd.Flags().GetBool("version"); err == nil && versionFlag {
				versionErr := e.NewVersionExec(&tmpConfig).Execute(false, "")
				if versionErr != nil {
					errUtils.CheckErrorPrintAndExit(versionErr, "", "")
				}
				// Use ExitCodeError{Code: 0} to exit cleanly without os.Exit() (untestable).
				// This follows the existing pattern for preserving exit codes.
				errUtils.CheckErrorPrintAndExit(errUtils.ExitCodeError{Code: 0}, "", "")
				return
			}
		}

		// Enable performance tracking if heatmap flag is set.
		// P95 latency tracking via HDR histogram is automatically enabled.
		if showHeatmap, _ := cmd.Flags().GetBool("heatmap"); showHeatmap {
			perf.EnableTracking(true)
		}

		// Print telemetry disclosure if needed (skip for completion commands and when CLI config not found).
		if !isCompletionCommand(cmd) && err == nil {
			telemetry.PrintTelemetryDisclosure()
		}

		// Initialize I/O context and global formatter after flag parsing.
		// This ensures flags like --no-color, --redirect-stderr, --mask are respected.
		// Initialize() sets the global iolib.Data and iolib.UI writers with masking enabled.
		if ioErr := iolib.Initialize(); ioErr != nil {
			errUtils.CheckErrorPrintAndExit(fmt.Errorf("failed to initialize I/O context: %w", ioErr), "", "")
		}
		ioCtx := iolib.GetContext()
		ui.InitFormatter(ioCtx)
		data.InitWriter(ioCtx)
		data.SetMarkdownRenderer(ui.Format) // Connect markdown rendering to data channel
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		// Stop profiler after command execution.
		if profilerServer != nil {
			if stopErr := profilerServer.Stop(); stopErr != nil {
				log.Error("Failed to stop profiler", "error", stopErr)
			}
		}

		// Show performance heatmap if enabled.
		// Use IsTrackingEnabled() to support commands with DisableFlagParsing.
		if perf.IsTrackingEnabled() {
			heatmapMode, _ := cmd.Flags().GetString("heatmap-mode")
			// Default to "bar" mode if empty (happens with DisableFlagParsing).
			if heatmapMode == "" {
				heatmapMode = "bar"
			}
			if err := displayPerformanceHeatmap(cmd, heatmapMode); err != nil {
				log.Error("Failed to display performance heatmap", "error", err)
			}
		}
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration.
		checkAtmosConfig()

		// Print a styled Atmos logo to the terminal.
		fmt.Println()
		err := tuiUtils.PrintStyledText("ATMOS")
		if err != nil {
			return err
		}

		err = e.ExecuteAtmosCmd()
		return err
	},
}

// SetupLogger configures the global logger based on the provided Atmos configuration.
//
//nolint:revive,cyclop // Function complexity is acceptable for logger configuration.
func SetupLogger(atmosConfig *schema.AtmosConfiguration) {
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

	// Get theme-aware log styles.
	var styles *log.Styles
	if atmosConfig.Settings.Terminal.IsColorEnabled(term.IsTTYSupportForStderr()) {
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

	log.SetStyles(styles)
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
		// Enrich the error with proper formatting for user-facing output.
		// The error from ParseLogLevel has format: "sentinel\nexplanation"
		// Extract the explanation text (everything after the first newline).
		errMsg := err.Error()
		parts := strings.SplitN(errMsg, "\n", 2)
		explanation := ""
		if len(parts) > 1 {
			explanation = parts[1]
		}

		// Build enriched error from the wrapped error, adding the explanation as a detail.
		// This ensures the formatter can extract both the sentinel message and the explanation.
		enrichedErr := errUtils.Build(log.ErrInvalidLogLevel).
			WithExplanation(explanation).
			Err()
		errUtils.CheckErrorPrintAndExit(enrichedErr, "", "")
	}
	log.Debug("Set", "logs-level", log.GetLevelString(), "logs-file", atmosConfig.Logs.File)
}

// cleanupLogFile closes the log file handle if it was opened.
func cleanupLogFile() {
	if logFileHandle != nil {
		// Flush any remaining log data before closing.
		if err := logFileHandle.Sync(); err != nil {
			// Don't use logger here as we're cleaning up the log file
			fmt.Fprintf(os.Stderr, "Warning: failed to sync log file: %v\n", err)
		}
		if err := logFileHandle.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close log file: %v\n", err)
		}
		logFileHandle = nil
	}
}

// Cleanup performs cleanup operations before the program exits.
// This should be called by main when the program is terminating.
func Cleanup() {
	cleanupLogFile()
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

// handleConfigInitError processes config initialization errors and enriches them for display.
// Returns nil if the error can be ignored (e.g., for version command), or an enriched error.
func handleConfigInitError(initErr error, atmosConfig *schema.AtmosConfiguration) error {
	if isVersionCommand() {
		// Version command should always work, even with invalid config.
		log.Debug("Warning: CLI configuration error (continuing for version command)", "error", initErr)
		return nil
	}

	if errors.Is(initErr, cfg.NotFound) {
		// Config not found is acceptable for some commands.
		return nil
	}

	// For invalid log level errors, enrich with explanation and markdown formatting.
	if errors.Is(initErr, log.ErrInvalidLogLevel) {
		// Extract explanation from error message (format: "sentinel\nexplanation").
		errMsg := initErr.Error()
		parts := strings.SplitN(errMsg, "\n", 2)
		explanation := ""
		if len(parts) > 1 {
			explanation = parts[1]
		}

		// Initialize markdown renderer even with partial config for error formatting.
		// This is safe because atmosConfig struct exists even if validation failed.
		errUtils.InitializeMarkdown(atmosConfig)

		// Build enriched error with explanation.
		return errUtils.Build(log.ErrInvalidLogLevel).
			WithExplanation(explanation).
			Err()
	}

	// Return other errors as-is.
	return initErr
}

// Execute adds all child commands to the root command and sets flags appropriately.
// Execute runs the root CLI command and performs one-time global startup tasks.
// It processes a leading --chdir before loading configuration, loads and wires the CLI
// configuration to subsystems, initializes markdown rendering and logging, registers
// custom commands and aliases (unless running the version command), executes the root
// command, captures telemetry, and handles unknown-command errors by showing usage.
// This function is invoked once from main.main.
func Execute() error {
	// CRITICAL: Process --chdir flag BEFORE loading config.
	// This ensures atmos.yaml is loaded from the correct directory when using --chdir.
	// We must process chdir early because aliases depend on the config, and the config
	// depends on the working directory.
	//
	// Note: We create a temporary command to parse flags because RootCmd hasn't been
	// fully initialized yet (custom commands/aliases not added). This is safe because
	// --chdir is a global persistent flag defined in init().
	if err := processEarlyChdirFlag(); err != nil {
		return err
	}

	// InitCliConfig finds and merges CLI configurations in the following order:
	// system dir, home dir, current dir, ENV vars, command-line arguments
	// Here we need the custom commands from the config.
	var initErr error
	atmosConfig, initErr = cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)

	// Set atmosConfig for commands that need access to config.
	version.SetAtmosConfig(&atmosConfig)
	devcontainer.SetAtmosConfig(&atmosConfig)
	themeCmd.SetAtmosConfig(&atmosConfig)

	if initErr != nil {
		// Handle config initialization errors based on command context.
		if err := handleConfigInitError(initErr, &atmosConfig); err != nil {
			return err
		}
	}

	// Initialize markdown renderers only if config loaded successfully
	// This prevents deep exits in InitializeMarkdown when config is invalid
	if initErr == nil {
		utils.InitializeMarkdown(&atmosConfig)
		errUtils.InitializeMarkdown(&atmosConfig)
	}

	// Set the log level for the charmbracelet/log package based on the atmosConfig.
	SetupLogger(&atmosConfig)

	var err error
	// If CLI configuration was found, process its custom commands and command aliases.
	// Skip processing for version command to ensure it always works, even if aliases
	// reference commands that don't exist in this version of Atmos.
	if initErr == nil && !isVersionCommand() {
		err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)
		if err != nil {
			return err
		}

		err = processCommandAliases(atmosConfig, atmosConfig.CommandAliases, RootCmd, true)
		if err != nil {
			return err
		}
	}

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

// isCompletionCommand checks if the current invocation is for shell completion.
// This includes both user-visible completion commands and Cobra's internal
// hidden completion commands (__complete, __completeNoDesc).
// It works with both direct CLI invocations and programmatic SetArgs() calls.
func isCompletionCommand(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}

	// Check the command name directly from the Cobra command
	// This works for both os.Args and SetArgs() invocations
	cmdName := cmd.Name()
	if cmdName == "completion" || cmdName == "__complete" || cmdName == "__completeNoDesc" {
		return true
	}

	// Also check for shell completion environment variables
	// Cobra sets these when generating completions
	//nolint:forbidigo // These are external shell variables, not Atmos config
	if os.Getenv("COMP_LINE") != "" || os.Getenv("_ARGCOMPLETE") != "" {
		return true
	}

	return false
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
	// Filter out functions with zero total time for cleaner output (for table).
	snap := perf.SnapshotTopFiltered("total", defaultTopFunctionsMax)
	// Unbounded snapshot for accurate summary metrics.
	fullSnap := perf.SnapshotTopFiltered("total", 0)

	// Calculate total CPU time (sum of all self-times) and parallelism from all tracked functions.
	var totalCPUTime time.Duration
	for _, r := range fullSnap.Rows {
		totalCPUTime += r.Total
	}
	elapsed := fullSnap.Elapsed
	var parallelism float64
	if elapsed > 0 {
		parallelism = float64(totalCPUTime) / float64(elapsed)
	} else {
		parallelism = 0
	}

	utils.PrintfMessageToTUI("\n=== Atmos Performance Summary ===\n")
	utils.PrintfMessageToTUI("Elapsed: %s | CPU Time: %s | Parallelism: ~%.1fx\n",
		elapsed.Truncate(time.Microsecond),
		totalCPUTime.Truncate(time.Microsecond),
		parallelism)
	utils.PrintfMessageToTUI("Functions: %d | Total Calls: %d\n\n", snap.TotalFuncs, snap.TotalCalls)
	utils.PrintfMessageToTUI("%-50s %6s %13s %13s %13s %13s\n", "Function", "Count", "CPU Time", "Avg", "Max", "P95")
	for _, r := range snap.Rows {
		p95 := "-"
		if r.P95 > 0 {
			p95 = heatmap.FormatDuration(r.P95)
		}
		utils.PrintfMessageToTUI("%-50s %6d %13s %13s %13s %13s\n",
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

// init initializes CLI wiring for the package: it registers built-in commands, adds template helpers, registers and binds global persistent flags and related environment variables, adjusts the version flag default, sets the custom usage template, and configures Cobra behavior for the root command.
// This prepares global flag precedence, environment bindings (color, mask, verbose, GitHub token), and other root-level CLI integrations before command execution.
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

	// Register all global flags as persistent flags using builder pattern.
	// Global flags are registered as persistent so they're inherited by all subcommands.
	// This provides:
	//   - Single source of truth for defaults (NewGlobalFlags())
	//   - Automatic environment variable binding
	//   - Consistent with other command builders
	//   - Testable flag precedence
	globalParser := flags.NewGlobalOptionsBuilder().Build()
	globalParser.RegisterPersistentFlags(RootCmd)
	if err := globalParser.BindToViper(viper.GetViper()); err != nil {
		log.Error("Failed to bind global flags to viper", "error", err)
	}

	// Special handling for version flag: clear DefValue for cleaner --help output.
	if versionFlag := RootCmd.PersistentFlags().Lookup("version"); versionFlag != nil {
		versionFlag.DefValue = ""
	}
	// Bind both ATMOS_FORCE_COLOR and CLICOLOR_FORCE to the same viper key (they are equivalent).
	if err := viper.BindEnv("force-color", "ATMOS_FORCE_COLOR", "CLICOLOR_FORCE"); err != nil {
		log.Error("Failed to bind ATMOS_FORCE_COLOR/CLICOLOR_FORCE environment variables", "error", err)
	}
	// Bind mask flag to Viper so viper.GetBool("mask") reads the flag value.
	if err := viper.BindPFlag("mask", RootCmd.PersistentFlags().Lookup("mask")); err != nil {
		log.Error("Failed to bind mask flag to Viper", "error", err)
	}
	// Bind mask flag to environment variable.
	if err := viper.BindEnv("mask", "ATMOS_MASK"); err != nil {
		log.Error("Failed to bind ATMOS_MASK environment variable", "error", err)
	}
	// Bind verbose flag to environment variable.
	if err := viper.BindEnv(verboseFlagName, "ATMOS_VERBOSE"); err != nil {
		log.Error("Failed to bind ATMOS_VERBOSE environment variable", "error", err)
	}

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
	styles := boa.DefaultStyles()
	b := boa.New(boa.WithStyles(styles))
	oldUsageFunc := RootCmd.UsageFunc()
	RootCmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return showFlagUsageAndExit(c, err)
	})
	RootCmd.SetUsageFunc(func(c *cobra.Command) error {
		if c.Use == "atmos" {
			return b.UsageFunc(c)
		}
		// Get actual arguments (handles DisableFlagParsing=true case).
		arguments := flags.GetActualArgs(c, os.Args)

		// IMPORTANT: Check if command has Args validator and args are valid.
		// If args pass validation, they're positional args, not unknown subcommands.
		// This prevents "Unknown command component1" errors for valid positional args.
		if len(arguments) > 0 {
			if err := flags.ValidateArgsOrNil(c, arguments); err == nil {
				// Args are valid positional arguments - show usage without "Unknown command" error
				showErrorExampleFromMarkdown(c, "")
				errUtils.Exit(1)
				return nil
			}
			// Args validation failed - fall through to show error with first arg as unknown command
		}

		showUsageAndExit(c, arguments)
		return nil
	})
	RootCmd.SetHelpFunc(func(command *cobra.Command, args []string) {
		contentName := strings.ReplaceAll(strings.ReplaceAll(command.CommandPath(), " ", "_"), "-", "_")
		if exampleContent, ok := examples[contentName]; ok {
			command.Example = exampleContent.Content
		}

		if !(Contains(os.Args, "help") || Contains(os.Args, "--help") || Contains(os.Args, "-h")) {
			// Get actual arguments (handles DisableFlagParsing=true case).
			arguments := flags.GetActualArgs(command, os.Args)
			showUsageAndExit(command, arguments)
		}
		// Print a styled Atmos logo to the terminal.
		if command.Use != "atmos" || command.Flags().Changed("help") {
			var buf bytes.Buffer
			var err error
			command.SetOut(&buf)
			fmt.Println()
			if term.IsTTYSupportForStdout() {
				err = tuiUtils.PrintStyledTextToSpecifiedOutput(&buf, "ATMOS")
			} else {
				err = tuiUtils.PrintStyledText("ATMOS")
			}
			if err != nil {
				errUtils.CheckErrorPrintAndExit(err, "", "")
			}

			if err := oldUsageFunc(command); err != nil {
				errUtils.CheckErrorPrintAndExit(err, "", "")
			}

			// Check if pager should be enabled based on flag, env var, or config.
			pagerEnabled := atmosConfig.Settings.Terminal.IsPagerEnabled()

			// Check if --pager flag was explicitly set.
			if pagerFlag, err := command.Flags().GetString("pager"); err == nil && pagerFlag != "" {
				// Handle --pager flag values using switch for better readability.
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

			pager := pager.NewWithAtmosConfig(pagerEnabled)
			if err := pager.Run("Atmos CLI Help", buf.String()); err != nil {
				log.Error("Failed to run pager", "error", err)
				errUtils.OsExit(1)
			}
		} else {
			fmt.Println()
			err := tuiUtils.PrintStyledText("ATMOS")
			errUtils.CheckErrorPrintAndExit(err, "", "")

			b.HelpFunc(command, args)
			if err := command.Usage(); err != nil {
				errUtils.CheckErrorPrintAndExit(err, "", "")
			}
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
