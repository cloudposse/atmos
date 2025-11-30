package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/spf13/viper"
)

// DisplayDocs displays component documentation directly through the terminal or
// through a pager (like less). The use of a pager is determined by the pagination value
// set in the CLI Settings for Atmos.
func DisplayDocs(componentDocs string, usePager bool) error {
	defer perf.Track(nil, "utils.DisplayDocs")()

	if !usePager {
		fmt.Println(componentDocs)
		return nil
	}

	// Try to get pager from Viper configuration first, then fall back to environment variable.
	pagerCmd := viper.GetString("pager")
	if pagerCmd == "" {
		pagerCmd = os.Getenv("PAGER") //nolint:forbidigo // Standard Unix PAGER variable for display purposes
	}
	if pagerCmd == "" {
		pagerCmd = "less -r"
	}

	// Trim whitespace and split into args.
	pagerCmd = strings.TrimSpace(pagerCmd)
	args := strings.Fields(pagerCmd)
	if len(args) == 0 {
		return errUtils.ErrInvalidPagerCommand
	}

	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec // User-specified PAGER command is intentional
	cmd.Stdin = strings.NewReader(componentDocs)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to execute pager: %w", err)
	}

	return nil
}
