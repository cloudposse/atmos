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
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

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
	}

	globalLogger.SetLevel(logLevel)

	// Show timestamp in non-CI environments
	if os.Getenv("CI") == "" && os.Getenv("GITHUB_ACTIONS") == "" {
		globalLogger.SetTimeFormat("15:04:05")
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
		viper.SetConfigName(".gotcha")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("$HOME")
	}

	// Read in environment variables that match
	viper.SetEnvPrefix("GOTCHA")
	viper.AutomaticEnv()

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		// Only log in debug mode to avoid noise
		if viper.GetString("log.level") == "debug" {
			fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
		}
	}

	// Re-initialize the logger after config is loaded to pick up any log level changes
	initGlobalLogger()
}

// Execute runs the root command.
func Execute() error {
	// Initialize the logger immediately so it's available for command creation
	initGlobalLogger()
	
	// Initialize configuration
	cobra.OnInitialize(initConfig)

	// Create root command using Cobra
	rootCmd := &cobra.Command{
		Use:   "gotcha [path]",
		Short: "A beautiful test runner for Go with real-time progress tracking",
		Long: `Gotcha is a sophisticated Go test runner that provides real-time progress tracking,
beautiful terminal output, and flexible result formatting. It transforms the Go testing
experience with intuitive visual feedback and comprehensive test result analysis.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Default behavior is to run stream command
			streamCmd := newStreamCmd(globalLogger)
			streamCmd.SetArgs(args)

			// Copy flag values from root to stream command
			cmd.Flags().VisitAll(func(flag *pflag.Flag) {
				if streamCmd.Flags().Lookup(flag.Name) != nil {
					streamCmd.Flags().Set(flag.Name, flag.Value.String())
				}
			})

			return streamCmd.Execute()
		},
		SilenceUsage:  true,
		SilenceErrors: false,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file (default is .gotcha.yaml)")
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")

	// Bind log-level flag to viper
	viper.BindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log-level"))

	// Add flags from stream command to root for convenience
	streamCmd := newStreamCmd(globalLogger)
	rootCmd.Flags().AddFlagSet(streamCmd.Flags())

	// Add subcommands
	rootCmd.AddCommand(streamCmd)
	rootCmd.AddCommand(newParseCmd(globalLogger))
	rootCmd.AddCommand(newVersionCmd(globalLogger))

	// Use Fang for beautiful CLI with signal handling
	ctx := context.Background()

	// Add signal handling
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		globalLogger.Debug("Received interrupt signal")
		cancel()
	}()

	// Run the command with Fang for proper error handling
	if err := fang.Execute(ctx, rootCmd); err != nil {
		// Check if it's a test failure error to get the correct exit code
		var testErr *testFailureError
		if errors.As(err, &testErr) {
			os.Exit(testErr.ExitCode())
		}
		return err
	}

	return nil
}
