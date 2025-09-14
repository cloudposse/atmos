package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	coveragePkg "github.com/cloudposse/atmos/tools/gotcha/internal/coverage"
	"github.com/cloudposse/atmos/tools/gotcha/internal/output"
	"github.com/cloudposse/atmos/tools/gotcha/internal/parser"
	"github.com/cloudposse/atmos/tools/gotcha/internal/tui"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/cache"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/stream"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/utils"
)

// newStreamCmd creates the stream subcommand.
func newStreamCmd(logger *log.Logger) *cobra.Command {
	streamCmd := &cobra.Command{
		Use:   "stream [path]",
		Short: "Stream test results as they execute",
		Long: `Execute go test and stream results in real-time.
This is the default command when running gotcha without arguments.`,
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
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStream(cmd, args, logger)
		},
	}

	// Test execution flags
	streamCmd.Flags().StringP("run", "r", "", "Run only tests matching regular expression")
	streamCmd.Flags().StringP("timeout", "t", "10m", "Test timeout")
	streamCmd.Flags().BoolP("short", "s", false, "Run smaller tests")
	streamCmd.Flags().Bool("race", false, "Enable race detector")
	streamCmd.Flags().Int("count", 1, "Run tests this many times")
	streamCmd.Flags().Bool("shuffle", false, "Shuffle test order")
	streamCmd.Flags().BoolP("verbose", "v", false, "Verbose output")

	// Coverage flags
	streamCmd.Flags().Bool("cover", false, "Enable coverage")
	streamCmd.Flags().String("coverprofile", "", "Coverage profile output file")
	streamCmd.Flags().String("coverpkg", "", "Apply coverage to packages matching this pattern")

	// Package selection flags
	streamCmd.Flags().String("include", "", "Include packages matching regex patterns (comma-separated)")
	streamCmd.Flags().String("exclude", "", "Exclude packages matching regex patterns (comma-separated)")

	// Output control flags
	streamCmd.Flags().StringP("show", "", "all", "Filter test results: all, failed, passed, skipped, collapsed, none")
	streamCmd.Flags().StringP("format", "f", "terminal", "Output format: terminal, json, markdown")
	streamCmd.Flags().StringP("output", "o", "", "Output file for test results")
	streamCmd.Flags().Bool("alert", false, "Sound alert when tests complete")

	// Verbosity flags
	streamCmd.Flags().String("verbosity", "standard", "Verbosity level: minimal, standard, with-output, verbose")

	// CI Integration flags
	streamCmd.Flags().Bool("ci", false, "CI mode - automatically detect and integrate with CI systems")
	streamCmd.Flags().String("post-comment", "", "GitHub PR comment posting strategy: always|never|adaptive|on-failure|on-skip|<os-name> (default: never)")
	streamCmd.Flags().String("github-token", "", "GitHub token for authentication (defaults to GITHUB_TOKEN env)")
	streamCmd.Flags().String("comment-uuid", "", "UUID for comment identification (defaults to GOTCHA_COMMENT_UUID env)")
	streamCmd.Flags().Bool("exclude-mocks", true, "Exclude mock files from coverage calculations")

	return streamCmd
}

// runStream executes the stream command.
// This function has been refactored from 324 lines to this simple delegation.
// The logic is now split across:
// - stream_config.go: Configuration extraction and validation
// - stream_execution.go: Test execution and output processing
// - stream_orchestrator.go: Main orchestration logic.
func runStream(cmd *cobra.Command, args []string, logger *log.Logger) error {
	return orchestrateStream(cmd, args, logger)
}

// runStreamOld is the original 324-line implementation preserved for reference.
// TODO: Remove this after verifying the refactored version works correctly.
func runStreamOld(cmd *cobra.Command, args []string, logger *log.Logger) error {
	// Parse test path
	testPath := "./..."
	if len(args) > 0 {
		testPath = args[0]
		logger.Debug("Test path specified", "path", testPath)
	}

	// Collect all the arguments for 'go test'
	var testArgs []string

	// Check for -- separator to allow raw go test args
	dashIndex := -1
	for i, arg := range os.Args {
		if arg == "--" {
			dashIndex = i
			break
		}
	}

	// If we have raw args after --, use them
	if dashIndex >= 0 && dashIndex < len(os.Args)-1 {
		testArgs = os.Args[dashIndex+1:]
		logger.Debug("Using raw go test arguments", "args", testArgs)
	} else {
		// Build args from flags
		// Note: We're building minimal args here. The full set would include all flags.
		if run, _ := cmd.Flags().GetString("run"); run != "" {
			testArgs = append(testArgs, "-run", run)
		}
		if timeout, _ := cmd.Flags().GetString("timeout"); timeout != "" && timeout != "10m" {
			testArgs = append(testArgs, "-timeout", timeout)
		}
		if short, _ := cmd.Flags().GetBool("short"); short {
			testArgs = append(testArgs, "-short")
		}
		if race, _ := cmd.Flags().GetBool("race"); race {
			testArgs = append(testArgs, "-race")
		}
		if count, _ := cmd.Flags().GetInt("count"); count > 1 {
			testArgs = append(testArgs, "-count", fmt.Sprintf("%d", count))
		}
		if shuffle, _ := cmd.Flags().GetBool("shuffle"); shuffle {
			testArgs = append(testArgs, "-shuffle", "on")
		}
	}

	// Get filter flags
	showFilter, _ := cmd.Flags().GetString("show")
	includePatterns, _ := cmd.Flags().GetString("include")
	excludePatterns, _ := cmd.Flags().GetString("exclude")

	// Validate show filter
	if !utils.IsValidShowFilter(showFilter) {
		return fmt.Errorf("%w: '%s' must be one of: all, failed, passed, skipped, collapsed, none", types.ErrInvalidShowFilter, showFilter)
	}

	// Get output settings
	format, _ := cmd.Flags().GetString("format")
	outputFile, _ := cmd.Flags().GetString("output")
	alert, _ := cmd.Flags().GetBool("alert")

	// Get verbosity level
	verbosityLevel, _ := cmd.Flags().GetString("verbosity")

	// Get coverage settings
	cover, _ := cmd.Flags().GetBool("cover")
	coverProfile, _ := cmd.Flags().GetString("coverprofile")
	coverPkg, _ := cmd.Flags().GetString("coverpkg")

	// Handle coverage flags
	if cover && coverProfile == "" {
		// Generate default coverage profile name with timestamp
		coverProfile = fmt.Sprintf("coverage-%s.out", time.Now().Format("20060102-150405"))
	}
	if coverProfile != "" {
		// Coverage is implicitly enabled if coverprofile is set
		cover = true
	}

	// Get CI settings
	ciMode, _ := cmd.Flags().GetBool("ci")

	// Bind flags to viper for environment variable support
	_ = viper.BindPFlag("post-comment", cmd.Flags().Lookup("post-comment"))
	_ = viper.BindEnv("post-comment", "GOTCHA_POST_COMMENT", "POST_COMMENT")
	postStrategy := viper.GetString("post-comment")

	_ = viper.BindPFlag("github-token", cmd.Flags().Lookup("github-token"))
	_ = viper.BindEnv("github-token", "GITHUB_TOKEN")

	_ = viper.BindPFlag("exclude-mocks", cmd.Flags().Lookup("exclude-mocks"))

	// Check if post-comment flag was actually set by the user
	postFlagPresent := cmd.Flags().Changed("post-comment") || viper.IsSet("post-comment")

	// Normalize the posting strategy
	postStrategy = normalizePostingStrategy(postStrategy, postFlagPresent)

	// Auto-detect CI mode if not explicitly set
	if !ciMode && config.IsCI() {
		ciMode = true
		logger.Debug("CI mode auto-detected",
			"CI", viper.GetBool("ci"),
			"GITHUB_ACTIONS", viper.GetBool("github.actions"))
	}

	// If CI mode and format is terminal, switch to a more appropriate format
	if ciMode && format == "terminal" {
		// Don't override if user explicitly set format
		if !cmd.Flags().Changed("format") {
			format = "markdown"
			logger.Debug("Switching to markdown format for CI mode")
		}
	}

	// Determine test packages
	var testPackages []string

	// Support various test path formats
	switch {
	case testPath == "./..." || testPath == "...":
		// Recursive from current directory
		testPackages = append(testPackages, "./...")
	case strings.HasSuffix(testPath, "/..."):
		// Recursive from specified directory
		testPackages = append(testPackages, testPath)
	case strings.Contains(testPath, ","):
		// Comma-separated list of packages
		for _, pkg := range strings.Split(testPath, ",") {
			testPackages = append(testPackages, strings.TrimSpace(pkg))
		}
	default:
		// Single package or directory
		testPackages = append(testPackages, testPath)
	}

	// Apply filters to packages
	filteredPackages, err := utils.FilterPackages(testPackages, includePatterns, excludePatterns)
	if err != nil {
		return err
	}

	if len(filteredPackages) == 0 {
		logger.Warn("No packages matched the filters")
		return nil
	}

	testPackages = filteredPackages

	// Debug: Log test packages
	logger.Debug("Test packages", "packages", testPackages)

	// Determine output file
	if outputFile == "" {
		outputFile = "test-output.json"
		if format == "markdown" {
			outputFile = "test-output.md"
		}
	}

	// Try to get an estimated test count
	estimatedTestCount := 0
	if !cmd.Flags().Changed("count") {
		// Only use cache if we're not running tests multiple times
		cacheManager, cacheErr := cache.NewManager(logger)
		if cacheErr == nil && cacheManager != nil {
			// Convert packages to pattern string for cache lookup
			pattern := strings.Join(testPackages, " ")
			if count, found := cacheManager.GetTestCount(pattern); found {
				estimatedTestCount = count
				logger.Debug("Using cached test count", "count", estimatedTestCount)
			} else {
				logger.Debug("No cached test count available")
			}
		}
	}

	// Override --show flag if --verbosity is specified
	if cmd.Flags().Changed("verbosity") {
		switch verbosityLevel {
		case "minimal":
			showFilter = "failed"
		case "standard":
			// Keep the existing show filter
		case "with-output":
			// Keep the existing show filter
		case "verbose":
			showFilter = "all"
		}
	}

	// Run tests based on output format and TTY availability
	var exitCode int

	if format == "terminal" && utils.IsTTY() {
		// Use interactive TUI mode
		logger.Debug("Starting interactive TUI mode")

		// Create the Bubble Tea model
		model := tui.NewTestModel(
			testPackages,
			strings.Join(testArgs, " "),
			outputFile,
			coverProfile,
			showFilter,
			alert,
			verbosityLevel,
			estimatedTestCount,
		)

		// Configure coverage options if needed
		if cover && coverPkg != "" {
			// Add coverage package configuration to test args
			// This would be passed through to the model
			logger.Debug("Coverage package filter", "coverpkg", coverPkg)
		}

		// Create Bubble Tea program
		p := tea.NewProgram(&model,
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
		)

		// Run the TUI
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("error running TUI: %w", err)
		}

		// Get the exit code from the model
		if m, ok := finalModel.(*tui.TestModel); ok {
			exitCode = m.GetExitCode()

			// Update cache with actual count if we have it
			if !m.IsAborted() && estimatedTestCount > 0 {
				// The model tracks actual test count during execution
				// We could enhance this to report back the actual count
				cacheManager, cacheErr := cache.NewManager(logger)
				// For now, just update the timestamp
				if cacheErr == nil && cacheManager != nil {
					pattern := strings.Join(testPackages, " ")
					_ = cacheManager.UpdateTestCount(pattern, estimatedTestCount, len(testPackages))
				}
			}
		}
	} else {
		// Use simple streaming mode (no TUI)
		logger.Debug("Starting simple streaming mode", "format", format)

		// For non-terminal formats or when not in a TTY
		exitCode = stream.RunSimpleStream(
			testPackages,
			strings.Join(testArgs, " "),
			outputFile,
			coverProfile,
			showFilter,
			alert,
			verbosityLevel,
		)
	}

	// Process results if needed
	if format != "terminal" || !utils.IsTTY() {
		// Read and parse the JSON output
		jsonData, err := os.ReadFile(outputFile)
		if err != nil {
			return fmt.Errorf("failed to read test output: %w", err)
		}

		// Parse the test events
		jsonReader := bytes.NewReader(jsonData)
		excludeMocks := viper.GetBool("exclude-mocks")
		summary, err := parser.ParseTestJSON(jsonReader, coverProfile, excludeMocks)
		if err != nil {
			return fmt.Errorf("failed to parse test output: %w", err)
		}

		// Note: Metadata fields would need to be added to types.TestSummary if needed

		// Handle different output formats
		switch format {
		case "json":
			if err := output.WriteSummary(summary, "json", outputFile); err != nil {
				return fmt.Errorf("failed to write JSON output: %w", err)
			}
			logger.Info("JSON output written", "file", outputFile)

		case "markdown":
			outputPath := strings.TrimSuffix(outputFile, filepath.Ext(outputFile)) + ".md"
			if err := output.WriteSummary(summary, "markdown", outputPath); err != nil {
				return fmt.Errorf("failed to write markdown output: %w", err)
			}
			logger.Info("Markdown output written", "file", outputPath)
		}

		// Handle CI comment posting if enabled
		logger.Debug("Checking if should post comment",
			"ciMode", ciMode,
			"postStrategy", postStrategy,
			"passed", len(summary.Passed),
			"failed", len(summary.Failed),
			"skipped", len(summary.Skipped))

		shouldPost := shouldPostComment(postStrategy, summary)
		logger.Debug("Should post decision", "shouldPost", shouldPost)

		if ciMode && shouldPost {
			logger.Info("Attempting to post GitHub comment", "strategy", postStrategy)
			// Post comment to CI system
			if err := postGitHubComment(summary, cmd, logger); err != nil {
				// Log error but don't fail the command
				logger.Error("Failed to post CI comment", "error", err)
			}
		} else {
			logger.Debug("Not posting comment", "ciMode", ciMode, "shouldPost", shouldPost)
		}
	}

	// Process coverage BEFORE checking exit code (as abort message appears after summary)
	// This ensures coverage analysis runs even if the process is marked as interrupted
	if coverProfile != "" {
		// Check if file exists first
		if _, err := os.Stat(coverProfile); err == nil {
			// Always show function coverage if we have a profile
			logger.Info("Analyzing coverage results...")
			if err := coveragePkg.ShowFunctionCoverageReport(coverProfile, logger); err != nil {
				logger.Debug("Function coverage unavailable", "error", err)
			}

			// Also process with config if available
			coverageConfig := getCoverageConfig()
			if coverageConfig.Enabled && (coverageConfig.Analysis.Functions || coverageConfig.Analysis.Statements) {
				if err := coveragePkg.ProcessCoverage(coverProfile, coverageConfig, logger); err != nil {
					logger.Debug("Coverage processing failed", "error", err)
				}
			}
		}
	}

	// Return with the test exit code
	if exitCode != 0 {
		return &testFailureError{code: exitCode}
	}

	return nil
}

// testFailureError is used to indicate test failures with specific exit codes.
type testFailureError struct {
	code        int
	testsFailed int
	testsPassed int
	reason      string
}

func (e *testFailureError) Error() string {
	// Provide a more descriptive error message based on the context.
	if e.reason != "" {
		return e.reason
	}
	
	// If no tests failed but exit code is non-zero, it's a process failure.
	if e.testsFailed == 0 && e.testsPassed > 0 {
		return fmt.Sprintf("test process failed with exit code %d (no test failures detected)", e.code)
	}
	
	// If tests failed, report that.
	if e.testsFailed > 0 {
		return fmt.Sprintf("tests failed with exit code %d", e.code)
	}
	
	// Generic fallback.
	return fmt.Sprintf("exit with code %d", e.code)
}

// getCoverageConfig retrieves the coverage configuration from viper.
func getCoverageConfig() config.CoverageConfig {
	var cfg config.CoverageConfig

	// Set defaults
	cfg.Enabled = viper.GetBool("coverage.enabled")
	cfg.Profile = viper.GetString("coverage.profile")

	// Analysis settings
	cfg.Analysis.Functions = viper.GetBool("coverage.analysis.functions")
	cfg.Analysis.Statements = viper.GetBool("coverage.analysis.statements")
	cfg.Analysis.Uncovered = viper.GetBool("coverage.analysis.uncovered")
	cfg.Analysis.Exclude = viper.GetStringSlice("coverage.analysis.exclude")

	// Output settings
	cfg.Output.Terminal.Format = viper.GetString("coverage.output.terminal.format")
	cfg.Output.Terminal.ShowUncovered = viper.GetInt("coverage.output.terminal.show_uncovered")

	// Threshold settings
	cfg.Thresholds.Total = viper.GetFloat64("coverage.thresholds.total")
	cfg.Thresholds.FailUnder = viper.GetBool("coverage.thresholds.fail_under")

	// If coverage isn't configured but we have a profile, enable basic analysis
	if !cfg.Enabled && cfg.Profile != "" {
		cfg.Enabled = true
		cfg.Analysis.Functions = true
		cfg.Analysis.Statements = true
	}

	return cfg
}

func (e *testFailureError) ExitCode() int {
	return e.code
}
