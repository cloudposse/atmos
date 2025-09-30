package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/spf13/viper"
)

func init() {
	// Bind PAGER environment variable with Atmos alternative.
	_ = viper.BindEnv("pager", "ATMOS_PAGER", "PAGER")
}

// DisplayDocs displays component documentation directly through the terminal or
// through a pager (like less). The use of a pager is determined by the pagination value
// set in the CLI Settings for Atmos.
func DisplayDocs(componentDocs string, usePager bool) error {
	if !usePager {
		fmt.Println(componentDocs)
		return nil
	}

	pagerCmd := viper.GetString("pager")
	if pagerCmd == "" {
		pagerCmd = "less -r"
	}

	args := strings.Fields(pagerCmd)
	if len(args) == 0 {
		return errUtils.ErrInvalidPagerCommand
	}

	//nolint:gosec // G204: Pager command is intentionally user-configurable via PAGER environment variable
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = strings.NewReader(componentDocs)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to execute pager: %w", err)
	}

	return nil
}
