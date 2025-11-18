package vhs

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// CheckInstalled verifies that VHS is available in the system PATH.
func CheckInstalled() error {
	defer perf.Track(nil, "vhs.CheckInstalled")()

	if _, err := exec.LookPath("vhs"); err != nil {
		return fmt.Errorf("%w (install: brew install vhs)", errUtils.ErrVHSNotFound)
	}
	return nil
}

// Render executes VHS to render a tape file in the specified working directory.
// Stdout and stderr are piped to the process's stdout/stderr for user visibility.
func Render(ctx context.Context, tapeFile, workingDir string) error {
	defer perf.Track(nil, "vhs.Render")()

	cmd := exec.CommandContext(ctx, "vhs", tapeFile)
	cmd.Dir = workingDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("vhs command failed: %w", err)
	}

	return nil
}
