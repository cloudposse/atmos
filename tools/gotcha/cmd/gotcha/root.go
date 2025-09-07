package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/fang"
	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/tools/gotcha/internal/markdown"
	"github.com/cloudposse/atmos/tools/gotcha/internal/output"
	"github.com/cloudposse/atmos/tools/gotcha/internal/parser"
	"github.com/cloudposse/atmos/tools/gotcha/internal/tui"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/cache"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/utils"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/ci"

	// Import CI integrations to register them
	_ "github.com/cloudposse/atmos/tools/gotcha/pkg/ci/github"
)

// Main package static errors.
var (
	ErrCommentUUIDRequired = errors.New("comment UUID is required")
)

// Global logger instance with consistent styling.
var globalLogger *log.Logger

// configFile holds the path to the config file if specified via --config flag.
var configFile string

// initGlobalLogger initializes the global logger with solid background colors per PRD spec.
func initGlobalLogger() {
	// Get the current color profile to preserve it
	profile := lipgloss.ColorProfile()

	globalLogger = log.New(os.Stderr)
	globalLogger.SetColorProfile(profile)
	globalLogger.SetStyles(&log.Styles{
		Levels: map[log.Level]lipgloss.Style{
			log.DebugLevel: lipgloss.NewStyle().
				SetString("DEBUG").
				Background(lipgloss.Color("#3F51B5")). // Indigo background
				Foreground(lipgloss.Color("#000000")). // Black foreground
				Padding(0, 1),
			log.InfoLevel: lipgloss.NewStyle().
				SetString("INFO").
				Background(lipgloss.Color("#4CAF50")). // Green background
				Foreground(lipgloss.Color("#000000")). // Black foreground
				Padding(0, 1),
			log.WarnLevel: lipgloss.NewStyle().
				SetString("WARN").
				Background(lipgloss.Color("#FF9800")). // Orange background
				Foreground(lipgloss.Color("#000000")). // Black foreground
				Padding(0, 1),
			log.ErrorLevel: lipgloss.NewStyle().
				SetString("ERROR").
				Background(lipgloss.Color("#F44336")). // Red background
				Foreground(lipgloss.Color("#000000")). // Black foreground
				Padding(0, 1),
			log.FatalLevel: lipgloss.NewStyle().
				SetString("FATAL").
				Background(lipgloss.Color("#F44336")). // Red background
				Foreground(lipgloss.Color("#FFFFFF")). // White foreground
				Padding(0, 1),
		},
		// Style the keys with a darker gray color
		Key: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")). // Dark gray for keys
			Bold(true),
		// Values stay with their default styling (no change)
		Value: lipgloss.NewStyle(),
		// Optional: style the separator between key and value
		Separator: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#999999")), // Medium gray for separator
	})

	// Get log level from configuration (flag > env > config > default)
	logLevelStr := viper.GetString("log.level")
	if logLevelStr == "" {
		logLevelStr = "info" // Default to info level
	}

	// Parse and set log level
	var logLevel log.Level
	switch strings.ToLower(logLevelStr) {
	case "debug":
		logLevel = log.DebugLevel
	case "info":
		logLevel = log.InfoLevel
	case "warn", "warning":
		logLevel = log.WarnLevel
	case "error":
		logLevel = log.ErrorLevel
	case "fatal":
		logLevel = log.FatalLevel
	default:
		logLevel = log.InfoLevel
		// Can't log warning yet as logger is being initialized
	}

	globalLogger.SetLevel(logLevel)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if configFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(configFile)
	} else {
		// Set config file name (without extension)
		viper.SetConfigName(".gotcha")
		viper.SetConfigType("yaml")

		// Search for config file in current directory
		viper.AddConfigPath(".")

		// Also search in parent directories
		viper.AddConfigPath("..")
		viper.AddConfigPath("../..")
		viper.AddConfigPath("../../..")
	}

	// Bind environment variables
	viper.SetEnvPrefix("GOTCHA")
	viper.AutomaticEnv()

	// Explicitly bind log.level to GOTCHA_LOG_LEVEL environment variable
	_ = viper.BindEnv("log.level", "GOTCHA_LOG_LEVEL")

	// Read config file if it exists (silently, logging happens after logger init)
	_ = viper.ReadInConfig()
}

// Execute is the main entry point for the cobra commands.
func Execute() error {
	// Pre-parse to get the config and log-level flags before full command execution
	// This allows us to load the config file and set log level before other processing
	var logLevel string
	for i, arg := range os.Args {
		if arg == "--config" && i+1 < len(os.Args) {
			configFile = os.Args[i+1]
		}
		// Also check for --config=value format
		if strings.HasPrefix(arg, "--config=") {
			configFile = strings.TrimPrefix(arg, "--config=")
		}
		// Check for log-level flag
		if arg == "--log-level" && i+1 < len(os.Args) {
			logLevel = os.Args[i+1]
		}
		// Also check for --log-level=value format
		if strings.HasPrefix(arg, "--log-level=") {
			logLevel = strings.TrimPrefix(arg, "--log-level=")
		}
	}

	// Initialize viper configuration
	initConfig()

	// Set log level from flag if provided (highest priority)
	if logLevel != "" {
		viper.Set("log.level", logLevel)
	}

	// Check for --no-color flag early
	var noColor bool
	for _, arg := range os.Args {
		if arg == "--no-color" {
			noColor = true
			break
		}
	}
	if noColor {
		viper.Set("no_color", true)
	}

	// Configure colors for lipgloss based on environment (GitHub Actions, CI, etc.)

	// Initialize global logger before any other operations
	initGlobalLogger()

	// Configure colors again and set logger color profile
	profile := tui.ConfigureColors()
	if globalLogger != nil {
		globalLogger.SetColorProfile(profile)
		globalLogger.Debug("Color profile configured",
			"profile", tui.ProfileName(profile),
			"github_actions", tui.IsGitHubActions(),
			"ci", tui.IsCI())

		// Log config file if one was loaded
		if viper.ConfigFileUsed() != "" {
			globalLogger.Debug("Loaded config file", "file", viper.ConfigFileUsed())
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Create root command
	rootCmd := &cobra.Command{
		Use:   "gotcha [path]",
		Short: "Advanced Go test runner with real-time progress tracking",
		Long: `Gotcha is a sophisticated Go test runner that provides real-time progress tracking,
beautiful terminal output, and flexible result formatting.

Run tests directly with streaming output, or process existing JSON results
from go test -json. Supports multiple output formats including GitHub
step summaries and markdown reports.`,
		Example: `  # Stream mode - run tests directly with real-time output
  gotcha
  gotcha .
  gotcha ./tests
  gotcha stream --packages="./..." --show=failed
  gotcha stream --packages="./pkg/..." --timeout=5m
  
  # Run specific tests using -run flag
  gotcha -- -run TestConfigLoad
  gotcha -- -run "TestConfig.*" -v
  gotcha stream -- -run TestStackProcess -race
  
  # Process existing JSON results  
  go test -json ./... | gotcha parse
  gotcha parse --input=test-results.json --format=markdown
  
  # Generate GitHub step summaries
  gotcha stream --format=github --output=step-summary.md
  
  # Advanced filtering and configuration
  gotcha stream --include=".*api.*" --exclude=".*mock.*" -- -race -short`,
		Args: cobra.MaximumNArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Bind the log-level flag to viper after Cobra has parsed it
			// Look in both local and persistent flags
			if flag := cmd.Flags().Lookup("log-level"); flag != nil {
				_ = viper.BindPFlag("log.level", flag)
			} else if flag := cmd.Root().PersistentFlags().Lookup("log-level"); flag != nil {
				_ = viper.BindPFlag("log.level", flag)
			}

			// Bind the no-color flag to viper after Cobra has parsed it
			if flag := cmd.Root().PersistentFlags().Lookup("no-color"); flag != nil {
				_ = viper.BindPFlag("no_color", flag)
				if viper.GetBool("no_color") {
					// Reconfigure colors when flag is set
					tui.ConfigureColors()
				}
			}

			// Re-apply log level if it was set via flag
			if logLevelStr := viper.GetString("log.level"); logLevelStr != "" {
				var logLevel log.Level
				switch strings.ToLower(logLevelStr) {
				case "debug":
					logLevel = log.DebugLevel
				case "info":
					logLevel = log.InfoLevel
				case "warn", "warning":
					logLevel = log.WarnLevel
				case "error":
					logLevel = log.ErrorLevel
				case "fatal":
					logLevel = log.FatalLevel
				default:
					logLevel = log.InfoLevel
					globalLogger.Warn("Invalid log level, using default", "level", logLevelStr, "default", "info")
				}
				globalLogger.SetLevel(logLevel)
				globalLogger.Debug("Log level updated", "level", logLevelStr)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Default to stream mode for zero-argument execution
			return runStream(cmd, args, globalLogger)
		},
	}

	// Add persistent flags (available to all subcommands)
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "Config file path (default: .gotcha.yaml)")
	rootCmd.PersistentFlags().String("log-level", "", "Log level: debug, info, warn, error, fatal (default: info)")
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable color output")

	// Add stream-specific flags to root command for direct usage
	rootCmd.Flags().String("packages", "", "Space-separated packages to test (default: ./...)")
	rootCmd.Flags().String("show", "all", "Filter displayed tests: all, failed, passed, skipped, collapsed, none")
	rootCmd.Flags().String("timeout", "40m", "Test timeout duration")
	rootCmd.Flags().String("output", "", "Output file for JSON results (default: test-results.json in temp dir)")
	rootCmd.Flags().String("coverprofile", "", "Coverage profile file for detailed analysis")
	rootCmd.Flags().Bool("exclude-mocks", true, "Exclude mock files from coverage calculations")
	rootCmd.Flags().String("include", ".*", "Regex patterns to include packages (comma-separated)")
	rootCmd.Flags().String("exclude", "", "Regex patterns to exclude packages (comma-separated)")
	rootCmd.Flags().BoolP("alert", "a", false, "Emit terminal bell when tests complete")
	rootCmd.Flags().String("verbosity", "standard", "Output verbosity: standard, with-output, minimal, or verbose")
	rootCmd.Flags().String("post-comment", "", "GitHub PR comment posting strategy: always|never|adaptive|on-failure|on-skip|<os-name> (default: always when flag present)")
	rootCmd.Flags().String("github-token", "", "GitHub token for authentication (defaults to GITHUB_TOKEN env)")
	rootCmd.Flags().String("comment-uuid", "", "UUID for comment identification (defaults to GOTCHA_COMMENT_UUID env)")

	// Add subcommands
	rootCmd.AddCommand(newStreamCmd(globalLogger))
	rootCmd.AddCommand(newParseCmd(globalLogger))
	rootCmd.AddCommand(newVersionCmd(globalLogger))

	// Use Fang to execute with proper signal handling
	return fang.Execute(ctx, rootCmd)
}

func newStreamCmd(logger *log.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stream [packages...]",
		Short: "Run tests directly with real-time streaming output",
		Long: `Stream mode runs Go tests directly and displays results in real-time
with beautiful progress bars, spinners, and colored output.

Automatically discovers the git repository root and runs tests from there.
Pre-calculates total test count for accurate progress tracking.`,
		Example: `  # Run all tests with default settings
  gotcha stream
  
  # Test specific packages  
  gotcha stream ./pkg/utils ./internal/...
  
  # Show only failed tests with custom timeout
  gotcha stream --show=failed --timeout=10m
  
  # Apply package filters
  gotcha stream --include=".*api.*" --exclude=".*mock.*"
  
  # Pass arguments to go test
  gotcha stream -- -race -short -count=3
  
  # Run specific tests using -run flag
  gotcha stream -- -run TestConfigLoad
  gotcha stream -- -run "TestConfig.*" -v
  gotcha stream --show=failed -- -run "Test.*Load" -race`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStream(cmd, args, globalLogger)
		},
	}

	// Add stream-specific flags
	cmd.Flags().String("packages", "", "Space-separated packages to test (default: ./...)")
	cmd.Flags().String("show", "all", "Filter displayed tests: all, failed, passed, skipped, collapsed, none")
	cmd.Flags().String("timeout", "40m", "Test timeout duration")
	cmd.Flags().String("output", "", "Output file for JSON results (default: test-results.json in temp dir)")
	cmd.Flags().String("coverprofile", "", "Coverage profile file for detailed analysis")
	cmd.Flags().Bool("exclude-mocks", true, "Exclude mock files from coverage calculations")
	cmd.Flags().String("include", ".*", "Regex patterns to include packages (comma-separated)")
	cmd.Flags().String("exclude", "", "Regex patterns to exclude packages (comma-separated)")
	cmd.Flags().BoolP("alert", "a", false, "Emit terminal bell when tests complete")
	cmd.Flags().String("verbosity", "standard", "Output verbosity: standard, with-output, minimal, or verbose")
	cmd.Flags().String("post-comment", "", "GitHub PR comment posting strategy: always|never|adaptive|on-failure|on-skip|<os-name> (default: always when flag present)")
	cmd.Flags().String("github-token", "", "GitHub token for authentication (defaults to GITHUB_TOKEN env)")
	cmd.Flags().String("comment-uuid", "", "UUID for comment identification (defaults to GOTCHA_COMMENT_UUID env)")

	return cmd
}

func newParseCmd(logger *log.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "parse [input-file]",
		Short: "Process existing JSON test results from go test -json",
		Long: `Parse mode processes JSON output from 'go test -json' and generates
formatted summaries, reports, and analysis.

Supports multiple output formats including markdown, GitHub step summaries,
and console output with rich formatting.`,
		Example: `  # Process results from stdin with terminal output
  go test -json ./... | gotcha parse
  
  # Process results from file  
  gotcha parse test-results.json
  gotcha parse --input=results.json --format=markdown
  
  # Generate GitHub step summary
  gotcha parse --format=github --output=step-summary.md
  
  # Terminal output plus markdown file
  gotcha parse --coverprofile=coverage.out --format=both`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runParse(cmd, args, logger)
		},
	}

	// Add parse-specific flags
	cmd.Flags().String("input", "", "Input file (JSON from go test -json). Use '-' or omit for stdin")
	cmd.Flags().String("format", "terminal", "Output format: terminal (console output), markdown (file), github (GitHub Actions), both (terminal+markdown)")
	cmd.Flags().String("output", "", "Output file (default: stdout for terminal/markdown)")
	cmd.Flags().String("coverprofile", "", "Coverage profile file for detailed analysis")
	cmd.Flags().Bool("exclude-mocks", true, "Exclude mock files from coverage calculations")
	cmd.Flags().String("post-comment", "", "GitHub PR comment posting strategy: always|never|adaptive|on-failure|on-skip|<os-name> (default: always when flag present)")
	cmd.Flags().Bool("generate-summary", false, "Write test summary to test-summary.md file")
	cmd.Flags().String("github-token", "", "GitHub token for authentication (defaults to GITHUB_TOKEN env)")
	cmd.Flags().String("comment-uuid", "", "UUID for comment identification (defaults to GOTCHA_COMMENT_UUID env)")

	return cmd
}

func newVersionCmd(logger *log.Logger) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("gotcha version 1.0.0")
			fmt.Println("A sophisticated Go test runner with real-time progress tracking")
			fmt.Println("Built with ❤️ using Charm libraries")
		},
	}
}

func runStream(cmd *cobra.Command, args []string, logger *log.Logger) error {
	logger.Info("Starting stream mode with real-time test execution")

	// Bind flags to viper
	_ = viper.BindPFlag("packages", cmd.Flags().Lookup("packages"))
	_ = viper.BindPFlag("show", cmd.Flags().Lookup("show"))
	_ = viper.BindPFlag("timeout", cmd.Flags().Lookup("timeout"))
	_ = viper.BindPFlag("output", cmd.Flags().Lookup("output"))
	_ = viper.BindPFlag("coverprofile", cmd.Flags().Lookup("coverprofile"))
	_ = viper.BindPFlag("exclude-mocks", cmd.Flags().Lookup("exclude-mocks"))
	_ = viper.BindPFlag("include", cmd.Flags().Lookup("include"))
	_ = viper.BindPFlag("exclude", cmd.Flags().Lookup("exclude"))
	_ = viper.BindPFlag("alert", cmd.Flags().Lookup("alert"))
	_ = viper.BindPFlag("verbosity", cmd.Flags().Lookup("verbosity"))

	// Get configuration values (from flags, env, or config file)
	packages := viper.GetString("packages")
	show := viper.GetString("show")
	timeout := viper.GetString("timeout")
	outputFile := viper.GetString("output")
	coverprofile := viper.GetString("coverprofile")
	include := viper.GetString("include")
	exclude := viper.GetString("exclude")
	alert := viper.GetBool("alert")
	verbosityLevel := viper.GetString("verbosity")
	// Default to with-output for backward compatibility
	if verbosityLevel == "" {
		verbosityLevel = "with-output"
	}

	// Validate show filter
	if !utils.IsValidShowFilter(show) {
		return fmt.Errorf("%w: '%s'. Valid options: all, failed, passed, skipped, collapsed, none", types.ErrInvalidShowFilter, show)
	}

	// Set default output file if empty
	if outputFile == "" {
		outputFile = filepath.Join(os.TempDir(), "test-results.json")
	}

	// Split args into packages and pass-through arguments
	// Everything after "--" should be passed directly to go test
	var testPackages []string
	var passthroughArgs []string

	// Check if cmd.ArgsLenAtDash() is set (indicates position of -- separator)
	dashPos := cmd.ArgsLenAtDash()
	if dashPos >= 0 {
		// We have a -- separator
		// Args before -- are packages, args after -- are pass-through
		if dashPos > 0 {
			args = args[:dashPos]
		} else {
			args = []string{}
		}
		// Cobra automatically puts everything after -- in args after dashPos
		// but since we sliced args above, we need to get them from os.Args
		// Find the -- position in os.Args and get everything after it
		for i, arg := range os.Args {
			if arg == "--" && i+1 < len(os.Args) {
				passthroughArgs = os.Args[i+1:]
				break
			}
		}
	}

	// Get packages from flags or arguments (before --)
	if packages != "" {
		testPackages = strings.Fields(packages)
	} else if len(args) > 0 {
		// Convert path arguments to Go package patterns
		testPackages = make([]string, 0, len(args))
		for _, arg := range args {
			if strings.HasSuffix(arg, "/...") {
				testPackages = append(testPackages, arg)
			} else if arg == "." {
				testPackages = append(testPackages, "./...")
			} else {
				// Normalize path and add recursive pattern
				if strings.HasSuffix(arg, "/") {
					arg = strings.TrimSuffix(arg, "/")
				}
				testPackages = append(testPackages, arg+"/...")
			}
		}
	} else {
		testPackages = []string{"./..."}
	}

	// Smart test name detection (non-intrusive, within existing flow)
	// Process testPackages to detect any test names that were passed as arguments
	detectedTestFilter := ""
	filteredTestPackages := []string{}

	for _, arg := range testPackages {
		// Check if this looks like a test name rather than a package path
		if utils.IsLikelyTestName(arg) {
			// Build up the test filter
			if detectedTestFilter != "" {
				detectedTestFilter += "|"
			}
			detectedTestFilter += arg
			logger.Debug("Detected test name in arguments", "test", arg)
		} else {
			// It's a package path
			filteredTestPackages = append(filteredTestPackages, arg)
		}
	}

	// If we detected test names but no packages, default to ./...
	if detectedTestFilter != "" && len(filteredTestPackages) == 0 {
		filteredTestPackages = []string{"./..."}
		logger.Info("Auto-detected test name(s), using default package path",
			"filter", detectedTestFilter, "path", "./...")
	}

	// If we have a test filter and no explicit -run flag, add it
	if detectedTestFilter != "" && !utils.HasRunFlag(passthroughArgs) {
		passthroughArgs = append([]string{"-run", detectedTestFilter}, passthroughArgs...)
		logger.Info("Auto-applying test filter", "filter", detectedTestFilter)
	}

	// Use the filtered test packages
	testPackages = filteredTestPackages

	// Apply package filtering
	filteredPackages, err := utils.FilterPackages(testPackages, include, exclude)
	if err != nil {
		return fmt.Errorf("error filtering packages: %w", err)
	}
	testPackages = filteredPackages

	// Prepare test arguments
	testArgsStr := "-timeout " + timeout

	// Add pass-through arguments
	if len(passthroughArgs) > 0 {
		testArgsStr += " " + strings.Join(passthroughArgs, " ")
		logger.Debug("Pass-through arguments detected", "args", passthroughArgs)
	}

	// Determine the actual test filter being used (from detected or explicit -run flag)
	var actualTestFilter string
	if detectedTestFilter != "" {
		actualTestFilter = detectedTestFilter
	} else {
		// Check for explicit -run flag in passthrough args
		for i := 0; i < len(passthroughArgs)-1; i++ {
			if passthroughArgs[i] == "-run" {
				actualTestFilter = passthroughArgs[i+1]
				break
			}
		}
	}

	// Initialize cache manager for test count estimation
	var cacheManager *cache.Manager
	var estimatedTestCount int
	packagePattern := strings.Join(testPackages, " ")

	// Try to use cache for test count estimation (enabled by default)
	// The cache manager will set its own defaults and check if explicitly disabled
	cacheManager, err = cache.NewManager(logger)
	if err != nil {
		logger.Info("Cache system disabled or unavailable", "reason", err.Error())
	} else if cacheManager != nil {
		// Try to get cached test count, considering any filter
		if count, ok := cacheManager.GetTestCountForFilter(packagePattern, actualTestFilter); ok {
			if actualTestFilter != "" {
				logger.Info("Found cached test count for filtered pattern",
					"pattern", packagePattern, "filter", actualTestFilter, "estimated_tests", count)
			} else {
				logger.Info("Found cached test count, using for progress estimation",
					"pattern", packagePattern, "estimated_tests", count)
			}
			estimatedTestCount = count
		} else {
			if actualTestFilter != "" {
				logger.Info("No cached test count found for filtered pattern",
					"pattern", packagePattern, "filter", actualTestFilter)
			} else {
				logger.Info("No cached test count found for pattern, will cache after run",
					"pattern", packagePattern, "cache_file", ".gotcha/cache.yaml")
			}
		}
	}

	// Log test patterns to be discovered
	logger.Info("Starting test execution", "patterns", len(testPackages))

	// Track test exit code and abort status for later return
	var testExitCode int
	var testsAborted bool

	// Check if we have a TTY for interactive mode
	logger.Debug("TTY detection", "is_tty", utils.IsTTY())
	if utils.IsTTY() {
		// Create and run the Bubble Tea program with estimated test count from cache
		model := tui.NewTestModel(testPackages, testArgsStr, outputFile, coverprofile, show, alert, verbosityLevel, estimatedTestCount)
		// Use default Bubble Tea configuration
		p := tea.NewProgram(&model)

		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("failed to run test UI: %w", err)
		}

		// Extract exit code and check if aborted after TUI exits
		if m, ok := finalModel.(*tui.TestModel); ok {
			// The TUI might have changed the global color profile
			// Re-detect and configure colors properly
			tui.ConfigureColors()

			// Re-initialize global logger after TUI exits with proper styles
			initGlobalLogger()
			// Use the reinitalized globalLogger for logging

			// Log info messages now that TUI is done
			_ = viper.BindEnv("GOTCHA_GITHUB_STEP_SUMMARY", "GITHUB_STEP_SUMMARY")
			githubSummary := viper.GetString("GOTCHA_GITHUB_STEP_SUMMARY")
			if githubSummary == "" {
				globalLogger.Info("GITHUB_STEP_SUMMARY not set (skipped)")
			} else {
				globalLogger.Info(fmt.Sprintf("GitHub step summary written to %s", githubSummary))
			}

			elapsed := m.GetElapsedTime()
			if elapsed > 0 {
				globalLogger.Info(fmt.Sprintf("Tests completed in %.2fs", elapsed.Seconds()))
			}

			testExitCode = m.GetExitCode()
			testsAborted = m.IsAborted()
		}
	} else {
		// Fallback to simple streaming for CI/non-TTY environments
		testExitCode = utils.RunSimpleStream(testPackages, testArgsStr, outputFile, coverprofile, show, alert, verbosityLevel)
	}

	if testExitCode == 0 {
		logger.Info("Stream mode completed successfully")
	} else {
		logger.Info("Tests completed with failures", "exit_code", testExitCode)
	}

	// Update cache with test results, but skip if tests were aborted
	if cacheManager != nil && outputFile != "" && !testsAborted {
		// Try to parse the JSON file to get test counts
		if inputFile, err := os.Open(outputFile); err == nil {
			func() {
				defer inputFile.Close()

				// Parse with coverage if available
				excludeMocks := viper.GetBool("exclude-mocks")
				if summary, err := parser.ParseTestJSON(inputFile, coverprofile, excludeMocks); err == nil && summary != nil {
					totalTests := len(summary.Passed) + len(summary.Failed) + len(summary.Skipped)
					packagesScanned := len(testPackages)

					// Update cache appropriately based on whether we had a filter
					if actualTestFilter == "" {
						// No filter - save the complete test list for future filtering
						testNames := make([]string, 0, totalTests)
						for _, test := range summary.Passed {
							testNames = append(testNames, test.Test)
						}
						for _, test := range summary.Failed {
							testNames = append(testNames, test.Test)
						}
						for _, test := range summary.Skipped {
							testNames = append(testNames, test.Test)
						}

						if err := cacheManager.UpdateTestList(packagePattern, testNames, packagesScanned); err != nil {
							logger.Debug("Failed to update test list cache", "error", err)
						} else {
							logger.Info("Updated test list cache", "pattern", packagePattern, "count", totalTests)
						}
					} else {
						// Had a filter - don't pollute the cache
						logger.Debug("Skipping cache update for filtered test run",
							"pattern", packagePattern, "filter", actualTestFilter, "count", totalTests)
					}

					// Always add run history, including failed runs
					runHistory := cache.RunHistory{
						ID:         fmt.Sprintf("run_%s", time.Now().Format(time.RFC3339)),
						Timestamp:  time.Now(),
						Pattern:    packagePattern,
						Total:      totalTests,
						Passed:     len(summary.Passed),
						Failed:     len(summary.Failed),
						Skipped:    len(summary.Skipped),
						DurationMs: int64(summary.TotalElapsedTime * 1000),
						Flags:      passthroughArgs,
					}
					if err := cacheManager.AddRunHistory(runHistory); err != nil {
						logger.Debug("Failed to add run history", "error", err)
					} else {
						logger.Debug("Added run history", "total", totalTests, "passed", len(summary.Passed), "failed", len(summary.Failed))
					}
				} else if err != nil {
					logger.Debug("Could not parse test results for cache update", "error", err)
				}
			}()
		} else {
			logger.Debug("Could not open test results file for cache update", "error", err)
		}
	} else if testsAborted {
		logger.Info("Tests aborted, skipping cache update")
	}

	// Handle GitHub comment posting if requested
	_ = viper.BindPFlag("post-comment", cmd.Flags().Lookup("post-comment"))
	_ = viper.BindEnv("post-comment", "GOTCHA_POST_COMMENT")
	postStrategy := viper.GetString("post-comment")
	flagPresent := cmd.Flags().Changed("post-comment") || viper.IsSet("post-comment")
	normalizedStrategy := normalizePostingStrategy(postStrategy, flagPresent)

	if outputFile != "" {
		// Parse the JSON file we just created (again for GitHub comment)
		if inputFile, err := os.Open(outputFile); err == nil {
			defer inputFile.Close()

			// Parse with coverage if available
			excludeMocks := viper.GetBool("exclude-mocks")
			if summary, err := parser.ParseTestJSON(inputFile, coverprofile, excludeMocks); err == nil {
				// Check if we should post based on strategy and results
				if shouldPostCommentWithOS(normalizedStrategy, summary, runtime.GOOS) {
					logger.Info("Processing results for GitHub comment",
						"strategy", normalizedStrategy,
						"failed", len(summary.Failed),
						"skipped", len(summary.Skipped))

					if err := postGitHubComment(summary, cmd, logger); err != nil {
						logger.Warn("Failed to post GitHub comment", "error", err)
					}
				} else {
					logger.Debug("Skipping GitHub comment based on strategy",
						"strategy", normalizedStrategy,
						"failed", len(summary.Failed),
						"skipped", len(summary.Skipped))
				}
			} else {
				logger.Warn("Failed to parse results for GitHub comment", "error", err)
			}
		} else if normalizedStrategy != "" && normalizedStrategy != "never" {
			logger.Warn("Failed to open results file for GitHub comment", "error", err)
		}
	}

	// Return error if tests failed, but only after cache update and GitHub comment
	if testExitCode != 0 {
		return fmt.Errorf("%w with exit code %d", types.ErrTestsFailed, testExitCode)
	}

	return nil
}

func runParse(cmd *cobra.Command, args []string, logger *log.Logger) error {
	logger.Info("Starting parse mode for JSON test results")

	// Bind flags to viper
	_ = viper.BindPFlag("input", cmd.Flags().Lookup("input"))
	_ = viper.BindPFlag("format", cmd.Flags().Lookup("format"))
	_ = viper.BindPFlag("output", cmd.Flags().Lookup("output"))
	_ = viper.BindPFlag("coverprofile", cmd.Flags().Lookup("coverprofile"))
	_ = viper.BindPFlag("exclude-mocks", cmd.Flags().Lookup("exclude-mocks"))
	_ = viper.BindPFlag("generate-summary", cmd.Flags().Lookup("generate-summary"))

	// Get configuration values (from flags, env, or config file)
	input := viper.GetString("input")
	format := viper.GetString("format")
	outputFile := viper.GetString("output")
	coverprofile := viper.GetString("coverprofile")
	excludeMocks := viper.GetBool("exclude-mocks")
	generateSummary := viper.GetBool("generate-summary")

	// Handle input source
	var inputReader *os.File
	var err error
	if len(args) > 0 {
		input = args[0]
	}

	if input == "" || input == "-" {
		inputReader = os.Stdin
	} else {
		inputReader, err = os.Open(input)
		if err != nil {
			return fmt.Errorf("error opening input file: %w", err)
		}
		defer inputReader.Close()
	}

	// Parse test JSON
	summary, err := parser.ParseTestJSON(inputReader, coverprofile, excludeMocks)
	if err != nil {
		return fmt.Errorf("error parsing test results: %w", err)
	}

	// Handle output
	err = output.HandleOutput(summary, format, outputFile, generateSummary)
	if err != nil {
		return fmt.Errorf("error writing output: %w", err)
	}

	// Bind flag to viper
	_ = viper.BindPFlag("post-comment", cmd.Flags().Lookup("post-comment"))
	_ = viper.BindEnv("post-comment", "GOTCHA_POST_COMMENT")

	// Handle GitHub comment posting
	postStrategy := viper.GetString("post-comment")
	flagPresent := cmd.Flags().Changed("post-comment") || viper.IsSet("post-comment")
	normalizedStrategy := normalizePostingStrategy(postStrategy, flagPresent)

	logger.Debug("GitHub comment posting decision",
		"raw_strategy", postStrategy,
		"flag_present", flagPresent,
		"normalized_strategy", normalizedStrategy,
		"os", runtime.GOOS,
		"failed", len(summary.Failed),
		"skipped", len(summary.Skipped),
		"should_post", shouldPostCommentWithOS(normalizedStrategy, summary, runtime.GOOS))

	if shouldPostCommentWithOS(normalizedStrategy, summary, runtime.GOOS) {
		logger.Info("Posting GitHub comment",
			"strategy", normalizedStrategy,
			"os", runtime.GOOS)
		if err := postGitHubComment(summary, cmd, logger); err != nil {
			logger.Warn("Failed to post GitHub comment", "error", err)
			// Don't fail the command, just warn
		}
	} else {
		logger.Debug("Skipping GitHub comment based on conditions",
			"strategy", normalizedStrategy,
			"os", runtime.GOOS,
			"failed", len(summary.Failed),
			"skipped", len(summary.Skipped))
	}

	// Silence unused variable warning
	_ = excludeMocks

	logger.Info("Parse mode completed successfully")
	return nil
}

// postGitHubComment posts test summary as a GitHub PR comment.
// normalizePostingStrategy normalizes the posting strategy value.
func normalizePostingStrategy(strategy string, flagPresent bool) string {
	// Trim spaces
	strategy = strings.TrimSpace(strategy)

	// Handle the special case where flag is present but empty
	if flagPresent && strategy == "" {
		return "always"
	}

	// Handle boolean aliases
	switch strings.ToLower(strategy) {
	case "true", "1", "yes":
		return "always"
	case "false", "0", "no":
		return "never"
	default:
		return strings.ToLower(strategy)
	}
}

// shouldPostComment determines if we should post a comment based on strategy and test results.
func shouldPostComment(strategy string, summary *types.TestSummary) bool {
	return shouldPostCommentWithOS(strategy, summary, runtime.GOOS)
}

// shouldPostCommentWithOS determines if we should post with explicit OS for testing.
func shouldPostCommentWithOS(strategy string, summary *types.TestSummary, goos string) bool {
	switch strategy {
	case "", "never":
		return false

	case "always":
		return true

	case "adaptive":
		// Linux always posts, others only on failures/skips
		if goos == "linux" {
			return true
		}
		return len(summary.Failed) > 0 || len(summary.Skipped) > 0

	case "on-failure", "onfailure":
		return len(summary.Failed) > 0

	case "on-skip", "onskip":
		return len(summary.Skipped) > 0

	default:
		// Check if it's an OS name (linux, darwin, windows)
		return strategy == goos
	}
}

func postGitHubComment(summary *types.TestSummary, cmd *cobra.Command, logger *log.Logger) error {
	// Detect CI integration
	integration := ci.DetectIntegration(logger)
	if integration == nil {
		logger.Info("Skipping comment posting", "reason", "no CI integration detected")
		return nil
	}

	// Detect CI context
	ctx, err := integration.DetectContext()
	if err != nil {
		logger.Info("Skipping comment posting",
			"reason", "CI context not detected",
			"provider", integration.Provider(),
			"error", err)
		return nil
	}

	if !ctx.IsSupported() {
		logger.Info("Skipping comment posting",
			"reason", "unsupported event type",
			"provider", integration.Provider(),
			"event", ctx.GetEventName())
		return nil
	}

	// Get job discriminator from environment
	_ = viper.BindEnv("job_discriminator", "GOTCHA_JOB_DISCRIMINATOR")
	jobDiscriminator := viper.GetString("job_discriminator")

	// Detect platform for display purposes
	platform := jobDiscriminator
	if platform == "" {
		// Try to detect from common CI environment variables for display
		if runner := os.Getenv("RUNNER_OS"); runner != "" {
			platform = strings.ToLower(runner)
		} else {
			platform = runtime.GOOS
		}
	}

	logger.Info("Posting CI comment",
		"ci_provider", integration.Provider(),
		"os_platform", platform,
		"failed", len(summary.Failed),
		"skipped", len(summary.Skipped),
		"passed", len(summary.Passed))

	// Bind flags to viper
	_ = viper.BindPFlag("github-token", cmd.Flags().Lookup("github-token"))
	_ = viper.BindPFlag("comment-uuid", cmd.Flags().Lookup("comment-uuid"))

	// Get UUID from viper (flag, env, or config) or context
	uuid := viper.GetString("comment-uuid")
	if uuid == "" {
		uuid = ctx.GetCommentUUID()
	}

	if uuid == "" {
		return fmt.Errorf("%w (use --comment-uuid flag or GOTCHA_COMMENT_UUID env)", ErrCommentUUIDRequired)
	}

	// Append job discriminator to UUID if present
	if jobDiscriminator != "" {
		uuid = fmt.Sprintf("%s-%s", uuid, jobDiscriminator)
		logger.Debug("Using discriminated UUID", "uuid", uuid, "discriminator", jobDiscriminator)
	}

	// Update the context with the discriminated UUID if it supports it
	// This is needed for GitHub to find existing comments with the discriminated UUID
	type contextWithUUID interface {
		ci.Context
		SetCommentUUID(string)
	}

	if ctxWithUUID, ok := ctx.(contextWithUUID); ok {
		ctxWithUUID.SetCommentUUID(uuid)
		// Update ctx to use the modified version
		ctx = ctxWithUUID
	}

	logger.Info("Posting comment",
		"platform", integration.Provider(),
		"owner", ctx.GetOwner(),
		"repo", ctx.GetRepo(),
		"pr", ctx.GetPRNumber(),
		"event", ctx.GetEventName(),
		"uuid", uuid)

	// Create comment manager
	commentManager := integration.CreateCommentManager(ctx, logger)
	if commentManager == nil {
		logger.Warn("Comment manager not available for platform", "platform", integration.Provider())
		return nil
	}

	// Generate adaptive markdown content that uses full content when possible
	// Include platform in the header for clarity
	markdownContent := markdown.GenerateAdaptiveComment(summary, uuid, platform)

	logger.Debug("Generated adaptive comment",
		"size", len(markdownContent),
		"limit", 65536,
		"using_full", len(markdownContent) <= 65536)

	// Post or update comment
	return commentManager.PostOrUpdateComment(context.Background(), ctx, markdownContent)
}
