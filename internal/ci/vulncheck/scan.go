package vulncheck

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	govulncheckBinary = "govulncheck"
	defaultPackages   = "./..."
	reportPermissions = 0o644
)

var errInvalidParams = errors.New("invalid vulncheck parameters")

// Params configures Scan.
type Params struct {
	// Output is the SARIF file to write. It is replaced only after the scan
	// succeeds and the report has been normalized.
	Output string
	// Packages are the package patterns to scan. It defaults to ./...
	Packages []string
	// Stderr receives govulncheck's diagnostics. It defaults to os.Stderr.
	Stderr io.Writer
}

// commandRunner runs name with args, writing to stdout and stderr. It is a seam
// so tests can supply govulncheck output without installing the tool.
type commandRunner func(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error

// Scan runs `govulncheck -format sarif` and writes the normalized report to
// params.Output. The tool must be on PATH. A failed scan, or output that is
// not a valid SARIF report, fails the scan and leaves any existing report
// untouched, so vulnerabilities can never be hidden by an empty upload.
func Scan(ctx context.Context, params *Params) error {
	defer perf.Track(nil, "vulncheck.Scan")()

	return scan(ctx, params, runCommand)
}

func scan(ctx context.Context, params *Params, run commandRunner) error {
	if params == nil || params.Output == "" {
		return fmt.Errorf("%w: output path is required", errInvalidParams)
	}
	packages := params.Packages
	if len(packages) == 0 {
		packages = []string{defaultPackages}
	}
	stderr := params.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	var raw bytes.Buffer
	args := append([]string{"-format", "sarif"}, packages...)
	if err := run(ctx, &raw, stderr, govulncheckBinary, args...); err != nil {
		return fmt.Errorf("run %s: %w", govulncheckBinary, err)
	}

	var normalized bytes.Buffer
	if err := DedupeStacks(&raw, &normalized); err != nil {
		return err
	}
	return writeFileAtomic(params.Output, normalized.Bytes())
}

func runCommand(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 -- fixed tool name, mage-target-controlled args.
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// writeFileAtomic writes data to a temporary file beside path and renames it
// into place, so a failure never leaves a truncated report behind.
func writeFileAtomic(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary report: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath) // No-op once the rename succeeds.

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write report: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	if err := os.Chmod(tempPath, reportPermissions); err != nil {
		return fmt.Errorf("set report permissions: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace report: %w", err)
	}
	return nil
}
