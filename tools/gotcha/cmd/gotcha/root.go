package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

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
	gh "github.com/cloudposse/atmos/tools/gotcha/pkg/github"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/utils"
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
	rootCmd.Flags().Bool("full-output", false, "Show complete output for failed tests with proper formatting")

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
	cmd.Flags().Bool("full-output", false, "Show complete output for failed tests with proper formatting")

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
		Example: `  # Process results from stdin
  go test -json ./... | gotcha parse
  
  # Process results from file  
  gotcha parse test-results.json
  gotcha parse --input=results.json --format=markdown
  
  # Generate GitHub step summary
  gotcha parse --format=github --output=step-summary.md
  
  # Include coverage analysis
  gotcha parse --coverprofile=coverage.out --format=both`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runParse(cmd, args, logger)
		},
	}

	// Add parse-specific flags
	cmd.Flags().String("input", "", "Input file (JSON from go test -json). Use '-' or omit for stdin")
	cmd.Flags().String("format", "stdin", "Output format: stdin, markdown, both, github")
	cmd.Flags().String("output", "", "Output file (default: stdout for stdin/markdown)")
	cmd.Flags().String("coverprofile", "", "Coverage profile file for detailed analysis")
	cmd.Flags().Bool("exclude-mocks", true, "Exclude mock files from coverage calculations")
	cmd.Flags().Bool("post-comment", false, "Post test summary as GitHub PR comment")
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
	_ = viper.BindPFlag("full-output", cmd.Flags().Lookup("full-output"))

	// Get configuration values (from flags, env, or config file)
	packages := viper.GetString("packages")
	show := viper.GetString("show")
	timeout := viper.GetString("timeout")
	outputFile := viper.GetString("output")
	coverprofile := viper.GetString("coverprofile")
	include := viper.GetString("include")
	exclude := viper.GetString("exclude")
	alert := viper.GetBool("alert")
	fullOutput := viper.GetBool("full-output")

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

	// Log test patterns to be discovered
	logger.Info("Starting test execution", "patterns", len(testPackages))

	// Check if we have a TTY for interactive mode
	logger.Debug("TTY detection", "is_tty", utils.IsTTY())
	if utils.IsTTY() {
		// Create and run the Bubble Tea program
		model := tui.NewTestModel(testPackages, testArgsStr, outputFile, coverprofile, show, alert, fullOutput)
		// Use default Bubble Tea configuration
		p := tea.NewProgram(&model)

		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("failed to run test UI: %w", err)
		}

		// Extract exit code and log info messages after TUI exits
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

			exitCode := m.GetExitCode()
			if exitCode != 0 {
				return fmt.Errorf("%w with exit code %d", types.ErrTestsFailed, exitCode)
			}
		}
	} else {
		// Fallback to simple streaming for CI/non-TTY environments
		exitCode := utils.RunSimpleStream(testPackages, testArgsStr, outputFile, coverprofile, show, alert, fullOutput)
		if exitCode != 0 {
			return fmt.Errorf("%w with exit code %d", types.ErrTestsFailed, exitCode)
		}
	}

	logger.Info("Stream mode completed successfully")
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

	// Handle GitHub comment posting
	postComment := viper.GetBool("post-comment")
	if postComment {
		if err := postGitHubComment(summary, cmd, logger); err != nil {
			logger.Warn("Failed to post GitHub comment", "error", err)
			// Don't fail the command, just warn
		}
	}

	// Silence unused variable warning
	_ = excludeMocks

	logger.Info("Parse mode completed successfully")
	return nil
}

// postGitHubComment posts test summary as a GitHub PR comment.
func postGitHubComment(summary *types.TestSummary, cmd *cobra.Command, logger *log.Logger) error {
	// Detect GitHub context
	ctx, err := gh.DetectContext()
	if err != nil {
		logger.Info("Skipping GitHub comment posting", "reason", "not in GitHub Actions context")
		return nil
	}

	if !ctx.IsSupported() {
		logger.Info("Skipping GitHub comment posting",
			"reason", "unsupported event type",
			"event", ctx.EventName)
		return nil
	}

	// Bind flags to viper
	_ = viper.BindPFlag("github-token", cmd.Flags().Lookup("github-token"))
	_ = viper.BindPFlag("comment-uuid", cmd.Flags().Lookup("comment-uuid"))

	// Get token from viper (flag, env, or config) or context
	token := viper.GetString("github-token")
	if token == "" {
		token = ctx.Token
	}

	// Get UUID from viper (flag, env, or config) or context
	uuid := viper.GetString("comment-uuid")
	if uuid == "" {
		uuid = ctx.CommentUUID
	}

	if uuid == "" {
		return fmt.Errorf("%w (use --comment-uuid flag or GOTCHA_COMMENT_UUID env)", ErrCommentUUIDRequired)
	}

	logger.Info("Posting GitHub comment",
		"owner", ctx.Owner,
		"repo", ctx.Repo,
		"pr", ctx.PRNumber,
		"event", ctx.EventName)

	// Create client and manager
	client := gh.NewClient(token)
	manager := gh.NewCommentManager(client, logger)

	// Generate markdown with UUID marker, using strategic resizing
	markdownContent := markdown.GenerateGitHubComment(summary, uuid)

	// Post or update comment
	return manager.PostOrUpdateComment(context.Background(), ctx, markdownContent)
}
