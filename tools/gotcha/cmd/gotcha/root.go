package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/charmbracelet/fang"
	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	// Import CI integrations to register them.
	_ "github.com/cloudposse/gotcha/pkg/ci/github"

	"github.com/cloudposse/gotcha/pkg/config"
)

// Main package static errors.
var (
	ErrCommentUUIDRequired = errors.New("comment UUID is required")
)

// Configuration and logging constants.
const (
	// Log levels.
	LogLevelDebug   = "debug"
	LogLevelInfo    = "info"
	LogLevelWarn    = "warn"
	LogLevelWarning = "warning"
	LogLevelError   = "error"
	LogLevelFatal   = "fatal"

	// Configuration.
	ConfigFileName = ".gotcha"
	ConfigFileType = "yaml"
	EnvPrefix      = "GOTCHA"

	// Force color values.
	ForceColorANSI      = "1"
	ForceColorANSI256   = "2"
	ForceColorTrueColor = "3"

	// Time format.
	TimeFormatShort = "15:04:05"
)

// exitError represents an error with a specific exit code.
type exitError struct {
	code int
}

func (e *exitError) Error() string {
	return fmt.Sprintf("exit with code %d", e.code)
}

func (e *exitError) ExitCode() int {
	return e.code
}

// Global logger instance with consistent styling.
var globalLogger *log.Logger

// configFile holds the path to the config file if specified via --config flag.
var configFile string

// parseLogLevel parses a log level string and returns the corresponding log.Level.
func parseLogLevel(levelStr string) log.Level {
	if levelStr == "" {
		levelStr = LogLevelInfo // Default to info level
	}

	switch strings.ToLower(levelStr) {
	case LogLevelDebug:
		return log.DebugLevel
	case LogLevelInfo:
		return log.InfoLevel
	case LogLevelWarn, LogLevelWarning:
		return log.WarnLevel
	case LogLevelError:
		return log.ErrorLevel
	case LogLevelFatal:
		return log.FatalLevel
	default:
		return log.InfoLevel
	}
}

// getLoggerStyles returns the logger styles configuration.
func getLoggerStyles() *log.Styles {
	return &log.Styles{
		Levels: map[log.Level]lipgloss.Style{
			log.DebugLevel: lipgloss.NewStyle().
				SetString("DEBUG").
				Background(lipgloss.Color("#3F51B5")). // Indigo background
				Foreground(lipgloss.Color("#000000")). // Black foreground
				Padding(0, 1, 0, 1),                   // Top, Right, Bottom, Left padding
			log.InfoLevel: lipgloss.NewStyle().
				SetString("INFO").
				Background(lipgloss.Color("#4CAF50")). // Green background
				Foreground(lipgloss.Color("#000000")). // Black foreground
				Padding(0, 1, 0, 1),                   // Top, Right, Bottom, Left padding
			log.WarnLevel: lipgloss.NewStyle().
				SetString("WARN").
				Background(lipgloss.Color("#FF9800")). // Orange background
				Foreground(lipgloss.Color("#000000")). // Black foreground
				Padding(0, 1, 0, 1),                   // Top, Right, Bottom, Left padding
			log.ErrorLevel: lipgloss.NewStyle().
				SetString("ERROR").
				Background(lipgloss.Color("#F44336")). // Red background
				Foreground(lipgloss.Color("#000000")). // Black foreground
				Padding(0, 1, 0, 1),                   // Top, Right, Bottom, Left padding
			log.FatalLevel: lipgloss.NewStyle().
				SetString("FATAL").
				Background(lipgloss.Color("#F44336")). // Red background
				Foreground(lipgloss.Color("#FFFFFF")). // White foreground
				Padding(0, 1, 0, 1),                   // Top, Right, Bottom, Left padding
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
	}
}

// initGlobalLogger initializes the global logger with solid background colors per PRD spec.
func initGlobalLogger() {
	// Force color profile based on environment
	var profile termenv.Profile

	// Check for forced color
	forceColor := viper.GetString("force.color")
	switch {
	case forceColor != "":
		switch forceColor {
		case ForceColorANSI:
			profile = termenv.ANSI
		case ForceColorANSI256:
			profile = termenv.ANSI256
		case ForceColorTrueColor:
			profile = termenv.TrueColor
		default:
			profile = termenv.ANSI256 // Default to 256 colors
		}
	case viper.GetBool("no.color"):
		profile = termenv.Ascii
	case viper.GetBool("github.actions"):
		// GitHub Actions supports 256 colors
		profile = termenv.ANSI256
	default:
		// Auto-detect
		profile = lipgloss.ColorProfile()
	}

	globalLogger = log.New(os.Stderr)
	globalLogger.SetColorProfile(profile)
	globalLogger.SetStyles(getLoggerStyles())

	// Parse and set log level
	globalLogger.SetLevel(parseLogLevel(viper.GetString("log.level")))

	// Show timestamp in non-CI environments
	if !config.IsCI() {
		globalLogger.SetTimeFormat(TimeFormatShort)
	} else {
		// In CI, don't show timestamps as CI systems add their own
		globalLogger.SetTimeFormat("")
	}
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if configFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(configFile)
	} else {
		// Search for config in standard locations
		viper.SetConfigName(ConfigFileName)
		viper.SetConfigType(ConfigFileType)
		viper.AddConfigPath(".")
		viper.AddConfigPath("$HOME")
	}

	// Read in environment variables that match
	viper.SetEnvPrefix(EnvPrefix)
	viper.AutomaticEnv()

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		// Only log in debug mode to avoid noise
		if viper.GetString("log.level") == LogLevelDebug {
			fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
		}
	}
}

// Execute runs the root command.
// createRootCommand creates and configures the root cobra command.
func createRootCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "gotcha [path]",
		Short: "A beautiful test runner for Go with real-time progress tracking",
		Long: `Gotcha is a sophisticated Go test runner that provides real-time progress tracking,
beautiful terminal output, and flexible result formatting. It transforms the Go testing
experience with intuitive visual feedback and comprehensive test result analysis.`,
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
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Simply delegate to runStream
			// This ensures viper config is properly respected
			return runStream(cmd, args, globalLogger)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
}

// setupGlobalFlags sets up global persistent flags.
func setupGlobalFlags(rootCmd *cobra.Command) {
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file (default is .gotcha.yaml)")
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")
}

// bindAndParseFlags binds viper flags and parses command line arguments.
func bindAndParseFlags(rootCmd *cobra.Command) error {
	// Bind log-level flag to viper BEFORE initializing logger
	// This ensures flag values override config file values
	if err := viper.BindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		// Can't use logger yet, it's not initialized
		fmt.Fprintf(os.Stderr, "Failed to bind log-level flag: %v\n", err)
	}

	// Parse flags early to get their values into viper
	// This is needed to ensure flag values override config file values
	if err := rootCmd.ParseFlags(os.Args[1:]); err != nil {
		// Can't use logger yet, it's not initialized
		fmt.Fprintf(os.Stderr, "Failed to parse flags: %v\n", err)
	}

	return nil
}

// setupSubcommands adds all subcommands to the root command.
func setupSubcommands(rootCmd *cobra.Command) {
	// Add flags from stream command to root for convenience
	streamCmd := newStreamCmd(globalLogger)
	rootCmd.Flags().AddFlagSet(streamCmd.Flags())

	// Add subcommands
	rootCmd.AddCommand(streamCmd)
	rootCmd.AddCommand(newParseCmd(globalLogger))
	rootCmd.AddCommand(newVersionCmd())
}

// createSignalContext creates a context with signal handling.
func createSignalContext() context.Context {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		globalLogger.Debug("Received interrupt signal")
		cancel()
	}()

	return ctx
}

func Execute() error {
	// Initialize environment configuration first to avoid os.Getenv usage
	config.InitEnvironment()

	// Initialize config BEFORE creating commands so viper values are available
	// This allows the config file to be read before command creation
	initConfig()

	// Create and configure root command
	rootCmd := createRootCommand()
	setupGlobalFlags(rootCmd)

	// Bind and parse flags
	if err := bindAndParseFlags(rootCmd); err != nil {
		return err
	}

	// NOW initialize the logger with the correct log level
	initGlobalLogger()

	// Setup subcommands
	setupSubcommands(rootCmd)

	// Create context with signal handling
	ctx := createSignalContext()

	// Run the command with Fang for proper error handling
	if err := fang.Execute(ctx, rootCmd); err != nil {
		// Return the error as-is so Fang can display it properly
		return err
	}

	return nil
}
