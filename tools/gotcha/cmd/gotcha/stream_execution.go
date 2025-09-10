package main

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

	internalLogger "github.com/cloudposse/atmos/tools/gotcha/internal/logger"
	"github.com/cloudposse/atmos/tools/gotcha/internal/output"
	"github.com/cloudposse/atmos/tools/gotcha/internal/parser"
	"github.com/cloudposse/atmos/tools/gotcha/internal/tui"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/cache"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/errors"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/stream"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/utils"
)

// runStreamInteractive runs tests in interactive TUI mode.
func runStreamInteractive(cmd *cobra.Command, config *StreamConfig, logger *log.Logger) (int, error) {
	// Set the global logger for packages that use it
	internalLogger.SetLogger(logger)

	logger.Debug("Confirmed running in TUI mode")
	logger.Debug("Starting interactive TUI mode")

	// Write to debug file if specified
	if debugFile := os.Getenv("GOTCHA_DEBUG_FILE"); debugFile != "" {
		if f, err := os.OpenFile(debugFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
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
	if os.Getenv("GOTCHA_TEST_MODE") == "true" {
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
		return 1, fmt.Errorf("error running TUI: %w", err)
	}

	// Get the exit code from the model
	var exitCode int
	if m, ok := finalModel.(*tui.TestModel); ok {
		exitCode = m.GetExitCode()

		// Print final summary
		summary := m.GenerateFinalSummary()
		if summary != "" {
			fmt.Fprint(os.Stderr, summary)
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
						logger.Warn("Updated test count cache", "pattern", pattern, "count", actualTestCount)
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

	return exitCode, nil
}

// runStreamInCI runs tests in CI mode (non-interactive).
func runStreamInCI(cmd *cobra.Command, config *StreamConfig, logger *log.Logger) (int, error) {
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
	if err := processTestOutput(config, cmd, logger); err != nil {
		return exitCode, err
	}

	return exitCode, nil
}

// processTestOutput processes test output for non-terminal formats.
func processTestOutput(config *StreamConfig, cmd *cobra.Command, logger *log.Logger) error {
	// Read and parse the JSON output
	jsonData, err := os.ReadFile(config.OutputFile)
	if err != nil {
		return fmt.Errorf("failed to read test output: %w", err)
	}

	// Parse the test events
	jsonReader := bytes.NewReader(jsonData)
	summary, err := parser.ParseTestJSON(jsonReader, config.CoverProfile, config.ExcludeMocks)
	if err != nil {
		return fmt.Errorf("failed to parse test output: %w", err)
	}

	// Handle different output formats
	if err := formatAndWriteOutput(summary, config, logger); err != nil {
		return err
	}

	// Handle CI comment posting if enabled
	if config.CIMode {
		return handleCICommentPosting(summary, config, cmd, logger)
	}

	return nil
}

// formatAndWriteOutput formats and writes test output based on format type.
func formatAndWriteOutput(summary *types.TestSummary, config *StreamConfig, logger *log.Logger) error {
	switch config.Format {
	case "json":
		if err := output.WriteSummary(summary, "json", config.OutputFile); err != nil {
			return fmt.Errorf("failed to write JSON output: %w", err)
		}
		logger.Info("JSON output written", "file", config.OutputFile)

	case "markdown":
		outputPath := strings.TrimSuffix(config.OutputFile, filepath.Ext(config.OutputFile)) + ".md"
		if err := output.WriteSummary(summary, "markdown", outputPath); err != nil {
			return fmt.Errorf("failed to write markdown output: %w", err)
		}
		logger.Info("Markdown output written", "file", outputPath)
	}

	return nil
}

// handleCICommentPosting handles posting comments to CI systems.
func handleCICommentPosting(summary *types.TestSummary, config *StreamConfig, cmd *cobra.Command, logger *log.Logger) error {
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

	return nil
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
	pattern := strings.Join(config.TestPackages, " ")

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
		logger.Debug("Using cached test count", "count", config.EstimatedTestCount, "pattern", pattern)
	} else {
		if testFilter != "" {
			logger.Info("No cached test count found for filtered pattern",
				"pattern", strings.Join(config.TestPackages, " "), "filter", testFilter)
		} else {
			logger.Info("No cached test count found for pattern, will cache after run",
				"pattern", pattern, "cache_file", ".gotcha/cache.yaml")
		}
	}
}
