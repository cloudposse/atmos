package cmd

import (
	"fmt"
	"os"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/utils"
)

// orchestrateStream coordinates the execution of the stream command.
// This function orchestrates configuration extraction, test preparation,
// and delegates to appropriate execution modes (TUI or CI).
func orchestrateStream(cmd *cobra.Command, args []string, logger *log.Logger) error {
	// Step 1: Extract and validate configuration
	config, err := extractStreamConfig(cmd, args, logger)
	if err != nil {
		return fmt.Errorf("failed to extract configuration: %w", err)
	}

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

	// Check for force-TUI mode
	forceTUI := os.Getenv("GOTCHA_FORCE_TUI") == "true"
	isTTY := utils.IsTTY()

	// Log mode selection decision
	logger.Debug("Mode selection",
		"format", config.Format,
		"isTTY", isTTY,
		"ciMode", config.CIMode,
		"forceTUI", forceTUI)

	// Support both "terminal" and "stream" formats for TUI mode (backward compatibility)
	// "stream" was the original format name for TUI with progress bar
	isTUIFormat := config.Format == "terminal" || config.Format == "stream"

	if (isTUIFormat && isTTY && !config.CIMode) || forceTUI {
		// Interactive TUI mode
		logger.Debug("Entering TUI mode",
			"forceTUI", forceTUI,
			"isTTY", isTTY,
			"format", config.Format,
			"reason", func() string {
				if forceTUI {
					return "GOTCHA_FORCE_TUI=true"
				}
				return "TTY detected with TUI format"
			}())
		exitCode, err = runStreamInteractive(cmd, config, logger)
	} else {
		// CI or non-interactive mode

		// Log when terminal/stream format is downgraded due to no TTY
		if isTUIFormat && !isTTY && !config.CIMode {
			logger.Info("Terminal format requested but no TTY detected, using non-interactive mode",
				"format", config.Format,
				"tip", "Run in an interactive terminal to see progress bar")
		} else if isTUIFormat && config.CIMode {
			logger.Info("Terminal format requested but CI mode detected, using non-interactive mode",
				"format", config.Format,
				"ciMode", config.CIMode)
		}

		logger.Debug("Entering non-TUI mode",
			"isTTY", isTTY,
			"format", config.Format,
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
		exitCode, err = runStreamInCI(cmd, config, logger)
	}

	if err != nil {
		return err
	}

	// Step 6: Exit with appropriate code
	if exitCode != 0 {
		// Return testFailureError to indicate test failure with specific exit code
		return &testFailureError{code: exitCode}
	}

	return nil
}
