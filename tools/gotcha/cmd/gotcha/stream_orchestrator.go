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

	// Step 5: Execute tests based on mode
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
	
	if (config.Format == "terminal" && isTTY && !config.CIMode) || forceTUI {
		// Interactive TUI mode
		logger.Debug("Entering TUI mode",
			"forceTUI", forceTUI,
			"isTTY", isTTY,
			"reason", func() string {
				if forceTUI {
					return "GOTCHA_FORCE_TUI=true"
				}
				return "TTY detected"
			}())
		exitCode, err = runStreamInteractive(cmd, config, logger)
	} else {
		// CI or non-interactive mode
		logger.Debug("Entering stream mode",
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
				if config.Format != "terminal" {
					return fmt.Sprintf("Format is %s", config.Format)
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
