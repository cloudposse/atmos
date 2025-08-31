package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/fang"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
)

// Global logger instance with consistent styling
var globalLogger *log.Logger

// initGlobalLogger initializes the global logger with solid background colors
func initGlobalLogger() {
	globalLogger = log.New(os.Stderr)
	globalLogger.SetLevel(log.InfoLevel)
	globalLogger.SetStyles(&log.Styles{
		Levels: map[log.Level]lipgloss.Style{
			log.DebugLevel: lipgloss.NewStyle().
				SetString("DEBUG").
				Background(lipgloss.Color("63")).
				Foreground(lipgloss.Color("0")).
				Padding(0, 1),
			log.InfoLevel: lipgloss.NewStyle().
				SetString("INFO").
				Background(lipgloss.Color("86")).
				Foreground(lipgloss.Color("0")).
				Padding(0, 1),
			log.WarnLevel: lipgloss.NewStyle().
				SetString("WARN").
				Background(lipgloss.Color("192")).
				Foreground(lipgloss.Color("0")).
				Padding(0, 1),
			log.ErrorLevel: lipgloss.NewStyle().
				SetString("ERROR").
				Background(lipgloss.Color("196")).
				Foreground(lipgloss.Color("0")).
				Padding(0, 1),
			log.FatalLevel: lipgloss.NewStyle().
				SetString("FATAL").
				Background(lipgloss.Color("196")).
				Foreground(lipgloss.Color("15")).
				Padding(0, 1),
		},
	})
}

func main() {
	// Initialize global logger before any other operations
	initGlobalLogger()
	
	if err := execute(); err != nil {
		os.Exit(1)
	}
}

func execute() error {

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Create root command
	var rootCmd = &cobra.Command{
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
  
  # Process existing JSON results  
  go test -json ./... | gotcha parse
  gotcha parse --input=test-results.json --format=markdown
  
  # Generate GitHub step summaries
  gotcha stream --format=github --output=step-summary.md
  
  # Advanced filtering and configuration
  gotcha stream --include=".*api.*" --exclude=".*mock.*" -- -race -short`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Default to stream mode for zero-argument execution
			return runStream(cmd, args, globalLogger)
		},
	}

	// Add stream-specific flags to root command for direct usage
	rootCmd.Flags().String("packages", "", "Space-separated packages to test (default: ./...)")
	rootCmd.Flags().String("show", "all", "Filter displayed tests: all, failed, passed, skipped")
	rootCmd.Flags().String("timeout", "40m", "Test timeout duration")
	rootCmd.Flags().String("output", "", "Output file for JSON results (default: gotcha-results.json)")
	rootCmd.Flags().String("coverprofile", "", "Coverage profile file for detailed analysis")
	rootCmd.Flags().Bool("exclude-mocks", true, "Exclude mock files from coverage calculations")
	rootCmd.Flags().String("include", ".*", "Regex patterns to include packages (comma-separated)")
	rootCmd.Flags().String("exclude", "", "Regex patterns to exclude packages (comma-separated)")

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
  gotcha stream -- -race -short -count=3`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStream(cmd, args, globalLogger)
		},
	}

	// Add stream-specific flags
	cmd.Flags().String("packages", "", "Space-separated packages to test (default: ./...)")
	cmd.Flags().String("show", "all", "Filter displayed tests: all, failed, passed, skipped")
	cmd.Flags().String("timeout", "40m", "Test timeout duration")
	cmd.Flags().String("output", "", "Output file for JSON results (default: gotcha-results.json)")
	cmd.Flags().String("coverprofile", "", "Coverage profile file for detailed analysis")
	cmd.Flags().Bool("exclude-mocks", true, "Exclude mock files from coverage calculations")
	cmd.Flags().String("include", ".*", "Regex patterns to include packages (comma-separated)")
	cmd.Flags().String("exclude", "", "Regex patterns to exclude packages (comma-separated)")

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
	
	// Get flags
	packages, _ := cmd.Flags().GetString("packages")
	show, _ := cmd.Flags().GetString("show")
	timeout, _ := cmd.Flags().GetString("timeout")
	output, _ := cmd.Flags().GetString("output")
	coverprofile, _ := cmd.Flags().GetString("coverprofile")
	_, _ = cmd.Flags().GetBool("exclude-mocks")  // Not used in stream mode
	include, _ := cmd.Flags().GetString("include")
	exclude, _ := cmd.Flags().GetString("exclude")
	
	// Set default output file if empty
	if output == "" {
		output = "gotcha-results.json"
	}
	
	// Get packages from flags or arguments
	var testPackages []string
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
	filteredPackages, err := filterPackages(testPackages, include, exclude)
	if err != nil {
		return fmt.Errorf("error filtering packages: %w", err)
	}
	testPackages = filteredPackages

	// Log package discovery
	logger.Info("Package discovery completed", "packages", len(testPackages))

	// Prepare test arguments
	testArgsStr := "-timeout " + timeout
	
	// Pre-calculate total test count for progress display
	totalTests := getTestCount(testPackages, testArgsStr)

	// Check if we have a TTY for interactive mode
	if isTTY() {
		// Create and run the Bubble Tea program
		model := newTestModel(testPackages, testArgsStr, output, coverprofile, show, totalTests)
		p := tea.NewProgram(model)
		
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("failed to run test UI: %w", err)
		}
		
		// Extract exit code from final model
		if m, ok := finalModel.(testModel); ok {
			exitCode := m.GetExitCode()
			if exitCode != 0 {
				return fmt.Errorf("tests failed with exit code %d", exitCode)
			}
		}
	} else {
		// Fallback to simple streaming for CI/non-TTY environments
		exitCode := runSimpleStream(testPackages, testArgsStr, output, coverprofile, show, totalTests)
		if exitCode != 0 {
			return fmt.Errorf("tests failed with exit code %d", exitCode)
		}
	}
	
	logger.Info("Stream mode completed successfully")
	return nil
}

func runParse(cmd *cobra.Command, args []string, logger *log.Logger) error {
	logger.Info("Starting parse mode for JSON test results")
	
	// Get flags
	input, _ := cmd.Flags().GetString("input")
	format, _ := cmd.Flags().GetString("format")
	output, _ := cmd.Flags().GetString("output")
	coverprofile, _ := cmd.Flags().GetString("coverprofile")
	excludeMocks, _ := cmd.Flags().GetBool("exclude-mocks")
	
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
	summary, err := parseTestJSON(inputReader, coverprofile, excludeMocks)
	if err != nil {
		return fmt.Errorf("error parsing test results: %w", err)
	}

	// Handle output
	err = handleOutput(summary, format, output)
	if err != nil {
		return fmt.Errorf("error writing output: %w", err)
	}
	
	// Silence unused variable warning
	_ = excludeMocks
	
	logger.Info("Parse mode completed successfully")
	return nil
}