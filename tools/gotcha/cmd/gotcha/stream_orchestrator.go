package cmd

import (
	"fmt"

	"github.com/charmbracelet/log"
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
	if config.Format == "terminal" && utils.IsTTY() && !config.CIMode {
		// Interactive TUI mode
		exitCode, err = runStreamInteractive(cmd, config, logger)
	} else {
		// CI or non-interactive mode
		exitCode, err = runStreamInCI(cmd, config, logger)
	}

	if err != nil {
		return err
	}

	// Step 6: Exit with appropriate code
	if exitCode != 0 {
		// Return error to indicate test failure
		return fmt.Errorf("tests failed with exit code %d", exitCode)
	}

	return nil
}