package testhelpers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	executablePerm = 0o755 // Permission for executable files.
)

// AtmosRunner manages running Atmos with optional coverage collection.
type AtmosRunner struct {
	binaryPath string
	coverDir   string
	buildOnce  sync.Once
	buildErr   error
}

// NewAtmosRunner creates a runner that builds Atmos with coverage if GOCOVERDIR is set.
func NewAtmosRunner(coverDir string) *AtmosRunner {
	return &AtmosRunner{
		coverDir: coverDir,
	}
}

// Build builds Atmos with coverage instrumentation if needed.
func (r *AtmosRunner) Build() error {
	r.buildOnce.Do(func() {
		if r.coverDir == "" {
			r.buildErr = r.useExistingBinary()
			return
		}
		// Ensure coverage directory exists.
		if err := os.MkdirAll(r.coverDir, executablePerm); err != nil {
			r.buildErr = fmt.Errorf("failed to create coverage directory: %w", err)
			return
		}
		r.buildErr = r.buildWithCoverage()
	})
	return r.buildErr
}

// buildWithCoverage builds Atmos with coverage instrumentation.
func (r *AtmosRunner) buildWithCoverage() error {
	tempBinary := filepath.Join(os.TempDir(), fmt.Sprintf("atmos-coverage-%d", os.Getpid()))
	// Build from the repository root.
	repoRoot, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("failed to find repository root: %w", err)
	}

	cmd := exec.Command("go", "build", "-cover", "-o", tempBinary, ".")
	cmd.Dir = repoRoot

	// Run the build command.
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to build atmos with coverage: %w\nOutput: %s", err, output)
	}

	// Make the binary executable.
	if err := os.Chmod(tempBinary, executablePerm); err != nil {
		return fmt.Errorf("failed to make binary executable: %w", err)
	}

	r.binaryPath = tempBinary
	return nil
}

// findBuildAtmos tries to find build/atmos in the git repository.
func findBuildAtmos() (string, bool) {
	wd, err := os.Getwd()
	if err != nil {
		return "", false
	}

	dir := wd
	for {
		gitDir := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			// Found git root, check for build/atmos.
			buildPath := filepath.Join(dir, "build", "atmos")
			if _, err := os.Stat(buildPath); err == nil {
				return buildPath, true
			}
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // Reached filesystem root.
		}
		dir = parent
	}
	return "", false
}

// useExistingBinary looks for an existing atmos binary, preferring build/atmos.
func (r *AtmosRunner) useExistingBinary() error {
	// Try to find build/atmos first.
	if buildPath, found := findBuildAtmos(); found {
		r.binaryPath = buildPath
		return nil
	}

	// Fall back to PATH.
	path, err := exec.LookPath("atmos")
	if err != nil {
		return fmt.Errorf("atmos not found in build/ or PATH: %w", err)
	}
	r.binaryPath = path
	return nil
}

// Command creates an exec.Cmd with GOCOVERDIR set.
func (r *AtmosRunner) Command(args ...string) *exec.Cmd {
	cmd := exec.Command(r.binaryPath, args...) //nolint:gosec // Binary path is controlled internally
	if r.coverDir != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("GOCOVERDIR=%s", r.coverDir))
	}
	return cmd
}

// CommandContext creates an exec.Cmd with context and GOCOVERDIR.
func (r *AtmosRunner) CommandContext(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, r.binaryPath, args...) //nolint:gosec // Binary path is controlled internally
	if r.coverDir != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("GOCOVERDIR=%s", r.coverDir))
	}
	return cmd
}

// BinaryPath returns the path to the binary being used.
func (r *AtmosRunner) BinaryPath() string {
	return r.binaryPath
}

// Cleanup removes temporary binary.
func (r *AtmosRunner) Cleanup() {
	if r.coverDir != "" && r.binaryPath != "" && filepath.Dir(r.binaryPath) == os.TempDir() {
		os.Remove(r.binaryPath)
	}
}

// findRepoRoot finds the root of the git repository.
func findRepoRoot() (string, error) {
	// Start from current directory and walk up to find .git directory.
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		gitDir := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached the root of the filesystem.
			return "", errUtils.ErrNoGitRepo
		}
		dir = parent
	}
}
