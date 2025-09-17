package cmd

import (
	"fmt"
	"os"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	coveragePkg "github.com/cloudposse/atmos/tools/gotcha/internal/coverage"
	"github.com/cloudposse/atmos/tools/gotcha/internal/tui"
	pkgConstants "github.com/cloudposse/atmos/tools/gotcha/pkg/constants"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/output"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/stream"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/utils"
)

// orchestrateStream coordinates the execution of the stream command.
// This function orchestrates configuration extraction, test preparation,
// and delegates to appropriate execution modes (TUI or CI).
//
// - Configuration extraction and validation
// - Test package preparation and filtering
// - Output format determination (TUI, stream, JSON, markdown)
// - CI mode detection and adaptation
// - Coverage report generation
// - GitHub integration (comments, job summaries)
// The complexity handles various execution modes and output requirements.
//
//nolint:nestif,gocognit // Stream orchestration coordinates multiple components:
func orchestrateStream(cmd *cobra.Command, args []string, logger *log.Logger, writer *output.Writer) error {
	// Step 1: Extract and validate configuration
	config, err := extractStreamConfig(cmd, args, logger)
	if err != nil {
		return fmt.Errorf("failed to extract configuration: %w", err)
	}

	// Set the writer in the config for downstream use
	config.Writer = writer

	// Step 2: Validate show filter
	if !utils.IsValidShowFilter(config.ShowFilter) {
		return fmt.Errorf("%w: '%s' must be one of: all, failed, passed, skipped, collapsed, none",
			types.ErrInvalidShowFilter, config.ShowFilter)
	}

	// Step 3: Prepare test packages
	if err := prepareTestPackages(config, logger); err != nil {
		return err
	}

	// Step 4: Load test count from cache
	loadTestCountFromCache(config, cmd, logger)

	// Step 5: Log what we'll be showing
	var filterDescription string
	switch config.ShowFilter {
	case "all":
		filterDescription = "all tests"
	case "failed":
		filterDescription = "failed and skipped tests only"
	case "passed":
		filterDescription = "passed tests only"
	case "skipped":
		filterDescription = "skipped tests only"
	case "none":
		filterDescription = "summary only (no individual tests)"
	default:
		filterDescription = config.ShowFilter
	}

	logger.Info("Test display configuration",
		"showing", filterDescription,
		"verbosity", config.VerbosityLevel,
		"packages", len(config.TestPackages),
	)

	// Step 6: Execute tests based on mode
	var exitCode int
	var testSummary *types.TestSummary // Needed to display detailed summary at the end

	// Check for force-TUI mode
	forceTUI := viper.GetBool("force.tui")
	isTTY := utils.IsTTY()

	// Log mode selection decision
	logger.Debug("Mode selection",
		pkgConstants.FormatField, config.Format,
		"isTTY", isTTY,
		"ciMode", config.CIMode,
		"forceTUI", forceTUI)

	// Support both "terminal" and "stream" formats for TUI mode (backward compatibility)
	// "stream" was the original format name for TUI with progress bar
	isTUIFormat := config.Format == pkgConstants.FormatTerminal || config.Format == "stream"

	if (isTUIFormat && isTTY && !config.CIMode) || forceTUI {
		// Interactive TUI mode
		logger.Debug("Entering TUI mode",
			"forceTUI", forceTUI,
			"isTTY", isTTY,
			pkgConstants.FormatField, config.Format,
			"reason", func() string {
				if forceTUI {
					return "GOTCHA_FORCE_TUI=true"
				}
				return "TTY detected with TUI format"
			}())
		exitCode, testSummary, err = runStreamInteractive(cmd, config, logger)
	} else {
		// CI or non-interactive mode

		// Log when terminal/stream format is downgraded due to no TTY
		if isTUIFormat && !isTTY && !config.CIMode {
			logger.Info("Terminal format requested but no TTY detected, using non-interactive mode",
				pkgConstants.FormatField, config.Format,
				"tip", "Run in an interactive terminal to see progress bar")
		} else if isTUIFormat && config.CIMode {
			logger.Info("Terminal format requested but CI mode detected, using non-interactive mode",
				pkgConstants.FormatField, config.Format,
				"ciMode", config.CIMode)
		}

		logger.Debug("Entering non-TUI mode",
			"isTTY", isTTY,
			pkgConstants.FormatField, config.Format,
			"ciMode", config.CIMode,
			"reason", func() string {
				if !isTTY {
					return "No TTY detected"
				}
				if config.CIMode {
					return "CI mode enabled"
				}
				if !isTUIFormat {
					return fmt.Sprintf("Format is %s (not terminal/stream)", config.Format)
				}
				return "Unknown"
			}())
		exitCode, testSummary, err = runStreamInCIWithSummary(cmd, config, logger)
	}

	if err != nil {
		return err
	}

	// Step 7: Display coverage and then comprehensive test summary at the very end
	// Process coverage first
	if config.CoverProfile != "" {
		// Check if file exists first
		if _, err := os.Stat(config.CoverProfile); err == nil {
			// Show divider before coverage
			config.Writer.PrintUI("\n%s\n", tui.GetDivider())

			// Always show function coverage if we have a profile
			logger.Info("Analyzing coverage results...")
			if err := coveragePkg.ShowFunctionCoverageReport(config.CoverProfile, config.Writer, logger); err != nil {
				logger.Debug("Function coverage unavailable", "error", err)
			}

			// Also process with config if available
			coverageConfig := getCoverageConfig()
			if coverageConfig.Enabled && (coverageConfig.Analysis.Functions || coverageConfig.Analysis.Statements) {
				if err := coveragePkg.ProcessCoverage(config.CoverProfile, coverageConfig, config.Writer, logger); err != nil {
					logger.Debug("Coverage processing failed", "error", err)
				}
			}
		}
	}

	// Note: Test summary is already displayed by the stream processor's PrintSummary() method
	// We don't call displayFinalTestSummary here to avoid duplicate summaries

	// Step 8: Exit with appropriate code
	if exitCode != 0 {
		testsFailed := 0
		testsPassed := 0
		if testSummary != nil {
			testsFailed = len(testSummary.Failed)
			testsPassed = len(testSummary.Passed)
		}

		// Log why we're exiting non-zero
		logger.Debug("Exiting with non-zero code",
			"exitCode", exitCode,
			"testsRun", func() int {
				if testSummary != nil {
					return len(testSummary.Passed) + len(testSummary.Failed) + len(testSummary.Skipped)
				}
				return 0
			}(),
			"testsFailed", testsFailed,
		)

		// Get the enhanced error reason from the stream processor
		exitReason := stream.GetLastExitReason()

		// Return testFailureError with context
		return &testFailureError{
			code:        exitCode,
			testsFailed: testsFailed,
			testsPassed: testsPassed,
			reason:      exitReason,
		}
	}

	logger.Debug("All tests passed, exiting with code 0")
	return nil
}

// Note: displayFinalTestSummary function was removed to avoid duplicate summaries.
// The test summary is displayed by the stream processor's PrintSummary() method.
