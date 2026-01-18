package vhs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

// CheckInstalled verifies that VHS is available in the system PATH.
func CheckInstalled() error {
	if _, err := exec.LookPath("vhs"); err != nil {
		return fmt.Errorf("%w (install: brew install vhs)", errUtils.ErrVHSNotFound)
	}
	return nil
}

// CheckSVGSupport verifies that the installed VHS supports SVG output.
// SVG support is indicated by the presence of the --no-svg-opt flag in help.
func CheckSVGSupport() error {
	cmd := exec.Command("vhs", "--help")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check VHS SVG support: %w", err)
	}

	// SVG support is indicated by the --no-svg-opt flag (available in agentstation/vhs fork).
	if !strings.Contains(string(output), "--no-svg-opt") {
		return fmt.Errorf("%w (install: brew install agentstation/tap/vhs)", errUtils.ErrVHSSVGNotSupported)
	}
	return nil
}

// ValidateTape runs VHS validate on a tape file to check for syntax errors.
// The tapeFile should be an absolute path to a preprocessed tape (with Source directives inlined).
// VHS runs from workdir so commands execute in the correct directory.
// Returns nil if the tape is valid, or an error with VHS validation output.
func ValidateTape(ctx context.Context, tapeFile, workdir string) error {
	// Run VHS validate from workdir. Tape file must be absolute path.
	cmd := exec.CommandContext(ctx, "vhs", "validate", tapeFile)
	cmd.Dir = workdir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tape validation failed:\n%s", string(output))
	}
	return nil
}

// Render executes VHS to render a tape file.
// The tapeFile should be an absolute path to a preprocessed tape (with Source directives inlined).
// VHS runs from workdir so commands execute in the correct directory.
// OutputDir is where VHS writes output files (passed as VHS_OUTPUT_DIR env var).
// Stdout and stderr are piped to the process's stdout/stderr for user visibility.
func Render(ctx context.Context, tapeFile, workdir, outputDir string) error {
	// Run VHS from workdir. Tape file must be absolute path.
	cmd := exec.CommandContext(ctx, "vhs", tapeFile)
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(),
		"VHS_OUTPUT_DIR="+outputDir,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("vhs command failed: %w", err)
	}

	return nil
}
