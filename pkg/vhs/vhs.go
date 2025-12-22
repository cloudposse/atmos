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

// Render executes VHS to render a tape file.
// Workdir is where VHS starts the shell (the scene's working directory).
// OutputDir is where VHS writes output files (passed as VHS_OUTPUT_DIR env var).
// Stdout and stderr are piped to the process's stdout/stderr for user visibility.
func Render(ctx context.Context, tapeFile, workdir, outputDir string) error {
	defer perf.Track(nil, "vhs.Render")()

	cmd := exec.CommandContext(ctx, "vhs", tapeFile)
	// VHS runs from workdir so the shell starts there (no cd commands needed).
	// VHS_OUTPUT_DIR env var is used by tape files for Output directives.
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(), "VHS_OUTPUT_DIR="+outputDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("vhs command failed: %w", err)
	}

	return nil
}
