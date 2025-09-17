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

	cmdConstants "github.com/cloudposse/gotcha/cmd/gotcha/constants"
	internalLogger "github.com/cloudposse/gotcha/internal/logger"
	"github.com/cloudposse/gotcha/internal/output"
	"github.com/cloudposse/gotcha/internal/parser"
	"github.com/cloudposse/gotcha/internal/tui"
	"github.com/cloudposse/gotcha/pkg/cache"
	pkgConstants "github.com/cloudposse/gotcha/pkg/constants"
	"github.com/cloudposse/gotcha/pkg/errors"
	"github.com/cloudposse/gotcha/pkg/stream"
	"github.com/cloudposse/gotcha/pkg/types"
	"github.com/cloudposse/gotcha/pkg/utils"
)

// runStreamInteractive runs tests in interactive TUI mode.
//
// - Test discovery must happen before TUI starts (users see accurate progress bars)
// - Cache checks must precede discovery (to avoid redundant work)
// - TUI model needs both discovered tests AND coverage config before initialization
// - Post-test cache updates depend on test results (can't be done earlier)
// - Coverage analysis requires completed tests AND valid profile file
// These operations have strict ordering dependencies that can't be parallelized
// or simplified without breaking the user experience (progress tracking, caching).
//
//nolint:nestif,gocognit,gocyclo // The complexity is necessary for coordinating async operations:
func runStreamInteractive(cmd *cobra.Command, config *StreamConfig, logger *log.Logger) (int, *types.TestSummary, error) {
	// Set the global logger for packages that use it
	internalLogger.SetLogger(logger)

	logger.Debug("Confirmed running in TUI mode")
	logger.Debug("Starting interactive TUI mode")

	// Write to debug file if specified
	if debugFile := viper.GetString("debug.file"); debugFile != "" {
		if f, err := os.OpenFile(debugFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, pkgConstants.DefaultFilePerms); err == nil {
			fmt.Fprintf(f, "\n=== TUI MODE STARTED ===\n")
			fmt.Fprintf(f, "Time: %s\n", time.Now().Format(time.RFC3339))
			fmt.Fprintf(f, "Packages: %v\n", config.TestPackages)
			fmt.Fprintf(f, "========================\n")
			f.Close()
		}
	}

	// Create the original TUI model
	model := tui.NewTestModel(
		config.TestPackages,
		strings.Join(config.TestArgs, " "),
		config.OutputFile,
		config.CoverProfile,
		config.ShowFilter,
		config.Alert,
		config.VerbosityLevel,
		config.EstimatedTestCount,
	)

	// Configure coverage options if needed
	if config.Cover && config.CoverPkg != "" {
		logger.Debug("Coverage package filter", "coverpkg", config.CoverPkg)
	}

	// Create Bubble Tea program options
	var opts []tea.ProgramOption

	// Check if we're in test mode (for AI or CI testing)
	if viper.GetBool("test.mode") {
		// Use WithoutRenderer for headless testing
		opts = append(opts, tea.WithoutRenderer())
		// Also provide nil input to avoid TTY requirements
		opts = append(opts, tea.WithInput(nil))
		logger.Debug("Running in test mode with WithoutRenderer and no input")
	}

	// Create Bubble Tea program without AltScreen to allow normal terminal scrolling
	p := tea.NewProgram(&model, opts...)

	// Run the TUI
	finalModel, err := p.Run()
	if err != nil {
		return 1, nil, fmt.Errorf("error running TUI: %w", err)
	}

	// Get the exit code and test summary from the model
	var exitCode int
	var testSummary *types.TestSummary
	if m, ok := finalModel.(*tui.TestModel); ok {
		exitCode = m.GetExitCode()

		// Print the single-line summary from TUI
		summary := m.GenerateFinalSummary()
		if summary != "" {
			fmt.Fprint(os.Stderr, summary)
		}

		// Build test summary for return
		testSummary = &types.TestSummary{
			Passed:   make([]types.TestResult, 0),
			Failed:   make([]types.TestResult, 0),
			Skipped:  make([]types.TestResult, 0),
			Coverage: "",
		}

		// Collect test results from package results
		for _, pkg := range m.GetPackageResults() {
			for testName, test := range pkg.Tests {
				testResult := types.TestResult{
					Package:    pkg.Package,
					Test:       testName,
					Status:     test.Status,
					Duration:   test.Elapsed,
					SkipReason: test.SkipReason,
				}

				switch test.Status {
				case "pass":
					testSummary.Passed = append(testSummary.Passed, testResult)
				case "fail":
					testSummary.Failed = append(testSummary.Failed, testResult)
				case "skip":
					testSummary.Skipped = append(testSummary.Skipped, testResult)
				}
			}
		}

		// Calculate average coverage
		totalCoverage := 0.0
		packageCount := 0
		for _, pkg := range m.GetPackageResults() {
			if pkg.StatementCoverage != "" && pkg.StatementCoverage != "0.0%" && pkg.StatementCoverage != "N/A" {
				var pct float64
				if _, err := fmt.Sscanf(pkg.StatementCoverage, "%f%%", &pct); err == nil {
					totalCoverage += pct
					packageCount++
				}
			} else if pkg.Coverage != "" && pkg.Coverage != "0.0%" {
				var pct float64
				if _, err := fmt.Sscanf(pkg.Coverage, "%f%%", &pct); err == nil {
					totalCoverage += pct
					packageCount++
				}
			}
		}
		if packageCount > 0 {
			testSummary.Coverage = fmt.Sprintf("%.1f%%", totalCoverage/float64(packageCount))
		}

		// Update cache with actual test count and package details if successful
		if !m.IsAborted() {
			actualTestCount := m.GetTotalTestCount()
			if actualTestCount > 0 {
				cacheManager, err := cache.NewManager(logger)
				if err != nil {
					logger.Error("Failed to create cache manager", "error", err)
				} else if cacheManager != nil {
					// Update total test count
					pattern := strings.Join(m.GetTestPackages(), " ")
					if err := cacheManager.UpdateTestCount(pattern, actualTestCount, len(m.GetTestPackages())); err != nil {
						logger.Error("Failed to update test count cache", "error", err)
					} else {
						logger.Debug("Updated test count cache", "pattern", pattern, "count", actualTestCount)
					}

					// Update per-package details
					packageResults := m.GetPackageResults()
					if len(packageResults) > 0 {
						packageDetails := make(map[string]cache.PackageDetail)
						for pkgName, pkgResult := range packageResults {
							testCount := 0
							for _, test := range pkgResult.Tests {
								if test != nil {
									testCount++
								}
							}
							packageDetails[pkgName] = cache.PackageDetail{
								TestCount:    testCount,
								LastModified: time.Now(),
							}
						}
						if err := cacheManager.UpdatePackageDetails(packageDetails); err != nil {
							logger.Error("Failed to update package details cache", "error", err)
						} else {
							logger.Debug("Updated package details cache", "packages", len(packageDetails))
						}
					}
				}
			}
		}
	}

	// Emit alert if requested
	utils.EmitAlert(config.Alert)

	return exitCode, testSummary, nil
}

// runStreamInCIWithSummary runs tests in CI mode and returns the test summary.
func runStreamInCIWithSummary(cmd *cobra.Command, config *StreamConfig, logger *log.Logger) (int, *types.TestSummary, error) {
	// Set the global logger for packages that use it
	internalLogger.SetLogger(logger)

	logger.Debug("Starting CI streaming mode", "format", config.Format)

	// Run tests in simple mode
	exitCode := stream.RunSimpleStream(
		config.TestPackages,
		strings.Join(config.TestArgs, " "),
		config.OutputFile,
		config.CoverProfile,
		config.ShowFilter,
		config.Alert,
		config.VerbosityLevel,
	)

	// Process and format output
	summary, err := processTestOutputWithSummary(config, cmd, logger)
	if err != nil {
		return exitCode, nil, err
	}

	return exitCode, summary, nil
}

// processTestOutputWithSummary processes test output and returns the summary.
func processTestOutputWithSummary(config *StreamConfig, cmd *cobra.Command, logger *log.Logger) (*types.TestSummary, error) {
	// Read and parse the JSON output
	jsonData, err := os.ReadFile(config.OutputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read test output: %w", err)
	}

	// Parse the test events
	jsonReader := bytes.NewReader(jsonData)
	summary, err := parser.ParseTestJSON(jsonReader, config.CoverProfile, config.ExcludeMocks)
	if err != nil {
		return nil, fmt.Errorf("failed to parse test output: %w", err)
	}

	// Handle different output formats
	if err := formatAndWriteOutput(summary, config, logger); err != nil {
		return summary, err
	}

	// Handle CI comment posting if enabled
	if config.CIMode {
		handleCICommentPosting(summary, config, cmd, logger)
	}

	return summary, nil
}

// formatAndWriteOutput formats and writes test output based on format type.
func formatAndWriteOutput(summary *types.TestSummary, config *StreamConfig, logger *log.Logger) error {
	switch config.Format {
	case cmdConstants.FormatJSON:
		if err := output.WriteSummary(summary, cmdConstants.FormatJSON, config.OutputFile); err != nil {
			return fmt.Errorf("failed to write JSON output: %w", err)
		}
		logger.Info("JSON output written", "file", config.OutputFile)

	case cmdConstants.FormatMarkdown:
		outputPath := strings.TrimSuffix(config.OutputFile, filepath.Ext(config.OutputFile)) + ".md"
		if err := output.WriteSummary(summary, cmdConstants.FormatMarkdown, outputPath); err != nil {
			return fmt.Errorf("failed to write markdown output: %w", err)
		}
		logger.Info("Markdown output written", "file", outputPath)

	case cmdConstants.FormatGitHub:
		outputPath := strings.TrimSuffix(config.OutputFile, filepath.Ext(config.OutputFile)) + ".md"
		if err := output.WriteSummary(summary, cmdConstants.FormatGitHub, outputPath); err != nil {
			return fmt.Errorf("failed to write github output: %w", err)
		}
		logger.Info("GitHub summary written", "file", outputPath)
	}

	return nil
}

// handleCICommentPosting handles posting comments to CI systems.
func handleCICommentPosting(summary *types.TestSummary, config *StreamConfig, cmd *cobra.Command, logger *log.Logger) {
	logger.Debug("Checking if should post comment",
		"ciMode", config.CIMode,
		"postStrategy", config.PostStrategy,
		"passed", len(summary.Passed),
		"failed", len(summary.Failed),
		"skipped", len(summary.Skipped))

	shouldPost := shouldPostComment(config.PostStrategy, summary)
	logger.Debug("Should post comment decision",
		"shouldPost", shouldPost,
		"strategy", config.PostStrategy)

	if config.CIMode && shouldPost {
		logger.Info("Attempting to post GitHub comment")
		if err := postGitHubComment(summary, cmd, logger); err != nil {
			logger.Error("Failed to post GitHub comment", "error", err)
			// Don't fail the command if comment posting fails
			// Just log the error and continue
		}
	}
}

// prepareTestPackages prepares and filters test packages.
func prepareTestPackages(config *StreamConfig, logger *log.Logger) error {
	// Parse test packages from path
	config.parseTestPackages()

	// Smart test name detection - check if any "packages" are actually test names
	detectedTestFilter := ""
	filteredTestPackages := []string{}

	for _, arg := range config.TestPackages {
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

	// If we detected test names, add them to the -run filter
	if detectedTestFilter != "" {
		// Check if there's already a -run filter
		hasRunFlag := false
		for i, arg := range config.TestArgs {
			if arg == "-run" && i+1 < len(config.TestArgs) {
				// Combine with existing filter
				config.TestArgs[i+1] = config.TestArgs[i+1] + "|" + detectedTestFilter
				hasRunFlag = true
				break
			}
		}

		// If no existing -run flag, add it
		if !hasRunFlag {
			config.TestArgs = append(config.TestArgs, "-run", detectedTestFilter)
		}

		// If no packages were specified, default to ./...
		if len(filteredTestPackages) == 0 {
			filteredTestPackages = []string{"./..."}
		}

		config.TestPackages = filteredTestPackages
		logger.Info("Detected test names, using filter", "filter", detectedTestFilter, "packages", filteredTestPackages)
	}

	// Apply filters to packages
	filteredPackages, err := utils.FilterPackages(
		config.TestPackages,
		config.IncludePatterns,
		config.ExcludePatterns,
	)
	if err != nil {
		return err
	}

	if len(filteredPackages) == 0 {
		logger.Warn("No packages matched the filters")
		return errors.ErrNoPackagesMatched
	}

	config.TestPackages = filteredPackages
	logger.Debug("Test packages", "packages", config.TestPackages)

	return nil
}

// loadTestCountFromCache loads estimated test count from cache.
func loadTestCountFromCache(config *StreamConfig, cmd *cobra.Command, logger *log.Logger) {
	// Only use cache if we're not running tests multiple times
	if cmd.Flags().Changed("count") {
		return
	}

	cacheManager, err := cache.NewManager(logger)
	if err != nil || cacheManager == nil {
		return
	}

	// Build cache key including test filter if present
	pattern := strings.Join(config.TestPackages, pkgConstants.SpaceString)

	// Check if there's a -run filter in test args
	testFilter := ""
	for i, arg := range config.TestArgs {
		if arg == "-run" && i+1 < len(config.TestArgs) {
			testFilter = config.TestArgs[i+1]
			break
		}
	}

	// Include filter in pattern for more accurate cache lookup
	if testFilter != "" {
		pattern = pattern + " -run " + testFilter
	}

	if count, found := cacheManager.GetTestCount(pattern); found {
		config.EstimatedTestCount = count
		logger.Debug("Using cached test count", "count", config.EstimatedTestCount, pkgConstants.PatternField, pattern)
	} else {
		if testFilter != "" {
			logger.Info("No cached test count found for filtered pattern",
				pkgConstants.PatternField, strings.Join(config.TestPackages, pkgConstants.SpaceString), "filter", testFilter)
		} else {
			logger.Info("No cached test count found for pattern, will cache after run",
				pkgConstants.PatternField, pattern, "cache_file", ".gotcha/cache.yaml")
		}
	}
}
