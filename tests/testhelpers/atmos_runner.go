package testhelpers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	executablePerm = 0o755 // Permission for executable files.
	windowsOS      = "windows"
)

// AtmosRunner manages running Atmos with optional coverage collection.
type AtmosRunner struct {
	binaryPath string
	coverDir   string
	buildOnce  sync.Once
	buildErr   error
}

// NewAtmosRunner creates a runner that always builds Atmos, with or without coverage instrumentation.
func NewAtmosRunner(coverDir string) *AtmosRunner {
	return &AtmosRunner{
		coverDir: coverDir,
	}
}

// Build always builds Atmos, with or without coverage instrumentation.
func (r *AtmosRunner) Build() error {
	r.buildOnce.Do(func() {
		if r.coverDir != "" {
			// Ensure coverage directory exists.
			if err := os.MkdirAll(r.coverDir, executablePerm); err != nil {
				r.buildErr = fmt.Errorf("failed to create coverage directory: %w", err)
				return
			}
			r.buildErr = r.buildWithCoverage()
		} else {
			r.buildErr = r.buildWithoutCoverage()
		}
	})
	return r.buildErr
}

// buildWithoutCoverage builds Atmos without coverage instrumentation.
func (r *AtmosRunner) buildWithoutCoverage() error {
	tempBinary := filepath.Join(os.TempDir(), fmt.Sprintf("atmos-test-%d", os.Getpid()))
	// Add .exe extension on Windows.
	if runtime.GOOS == windowsOS {
		tempBinary += ".exe"
	}
	// Build from the repository root.
	repoRoot, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("failed to find repository root: %w", err)
	}

	cmd := exec.Command("go", "build", "-o", tempBinary, ".")
	cmd.Dir = repoRoot

	// Run the build command.
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to build atmos: %w\nOutput: %s", err, output)
	}

	// Make the binary executable (Unix only, Windows doesn't need this).
	if runtime.GOOS != windowsOS {
		if err := os.Chmod(tempBinary, executablePerm); err != nil {
			return fmt.Errorf("failed to make binary executable: %w", err)
		}
	}

	r.binaryPath = tempBinary
	return nil
}

// buildWithCoverage builds Atmos with coverage instrumentation.
func (r *AtmosRunner) buildWithCoverage() error {
	tempBinary := filepath.Join(os.TempDir(), fmt.Sprintf("atmos-coverage-%d", os.Getpid()))
	// Add .exe extension on Windows.
	if runtime.GOOS == windowsOS {
		tempBinary += ".exe"
	}
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

	// Make the binary executable (Unix only, Windows doesn't need this).
	if runtime.GOOS != windowsOS {
		if err := os.Chmod(tempBinary, executablePerm); err != nil {
			return fmt.Errorf("failed to make binary executable: %w", err)
		}
	}

	r.binaryPath = tempBinary
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
	if r.coverDir != "" && r.binaryPath != "" {
		// Check if binary is in temp directory by checking if it starts with temp dir path.
		tempDir := os.TempDir()
		binaryDir := filepath.Dir(r.binaryPath)
		// Clean paths for comparison.
		tempDir = filepath.Clean(tempDir)
		binaryDir = filepath.Clean(binaryDir)
		if binaryDir == tempDir || strings.HasPrefix(binaryDir, tempDir+string(filepath.Separator)) {
			os.Remove(r.binaryPath)
		}
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
