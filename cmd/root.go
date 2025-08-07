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
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/elewis787/boa"
	"github.com/spf13/cobra"

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
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/toolchain"

	// Import built-in command packages for side-effect registration.
	// The init() function in each package registers the command with the registry.
	_ "github.com/cloudposse/atmos/cmd/about"
	"github.com/cloudposse/atmos/cmd/internal"
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
				errUtils.OsExit(0)
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
		if isVersionCommand() {
			log.Debug("Warning: CLI configuration 'atmos.yaml' file not found", "error", initErr)
		} else {
			return initErr
		}
	}

	// Set the log level for the charmbracelet/log package based on the atmosConfig.
	setupLogger(&atmosConfig)

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
	// Set custom usage template.

	RootCmd.AddCommand(toolchain.ToolChainCmd)
	// Set custom usage template
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
		showUsageAndExit(c, c.Flags().Args())
		return nil
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
