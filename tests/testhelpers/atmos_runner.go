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
	envpkg "github.com/cloudposse/atmos/pkg/env"
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
	// Create a unique temp directory for this test process
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("atmos-test-%d", os.Getpid()))
	if err := os.MkdirAll(tempDir, executablePerm); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Build the binary with the standard name 'atmos' so shell commands can find it
	tempBinary := filepath.Join(tempDir, "atmos")
	// Add .exe extension on Windows.
	if runtime.GOOS == windowsOS {
		tempBinary += ".exe"
	}
	// Build from the repository root.
	repoRoot, err := FindRepoRoot()
	if err != nil {
		return fmt.Errorf("failed to find repository root: %w", err)
	}

	// Use -buildvcs=false to support git worktrees where VCS stamping fails.
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", tempBinary, ".")
	cmd.Dir = repoRoot

	// Run the build command.
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to build atmos: %w\nOutput: %s", err, output)
	}

	// Verify binary was created
	if _, err := os.Stat(tempBinary); err != nil {
		return fmt.Errorf("binary not created at %s: %w", tempBinary, err)
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
	// Create a unique temp directory for this test process
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("atmos-coverage-%d", os.Getpid()))
	if err := os.MkdirAll(tempDir, executablePerm); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Build the binary with the standard name 'atmos' so shell commands can find it
	tempBinary := filepath.Join(tempDir, "atmos")
	// Add .exe extension on Windows.
	if runtime.GOOS == windowsOS {
		tempBinary += ".exe"
	}
	// Build from the repository root.
	repoRoot, err := FindRepoRoot()
	if err != nil {
		return fmt.Errorf("failed to find repository root: %w", err)
	}

	// Use -buildvcs=false to support git worktrees where VCS stamping fails.
	cmd := exec.Command("go", "build", "-buildvcs=false", "-cover", "-o", tempBinary, ".")
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

// Command creates an exec.Cmd with GOCOVERDIR and PATH set for nested atmos calls.
func (r *AtmosRunner) Command(args ...string) *exec.Cmd {
	cmd := exec.Command(r.binaryPath, args...) //nolint:gosec // Binary path is controlled internally
	cmd.Env = r.prepareEnvironment()
	// Inherit current working directory
	if wd, err := os.Getwd(); err == nil {
		cmd.Dir = wd
	}
	return cmd
}

// CommandContext creates an exec.Cmd with context, GOCOVERDIR and PATH set for nested atmos calls.
func (r *AtmosRunner) CommandContext(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, r.binaryPath, args...) //nolint:gosec // Binary path is controlled internally
	cmd.Env = r.prepareEnvironment()
	// Inherit current working directory
	if wd, err := os.Getwd(); err == nil {
		cmd.Dir = wd
	}
	return cmd
}

// prepareEnvironment sets up the environment for atmos commands with proper PATH and GOCOVERDIR.
func (r *AtmosRunner) prepareEnvironment() []string {
	env := os.Environ()

	// Ensure test binary is in PATH using testable utility functions
	updatedEnv := envpkg.EnsureBinaryInPath(env, r.binaryPath)

	// Handle GOCOVERDIR based on coverage settings
	// First filter out any existing GOCOVERDIR entries
	var filteredEnv []string
	for _, envVar := range updatedEnv {
		if !strings.HasPrefix(envVar, "GOCOVERDIR=") {
			filteredEnv = append(filteredEnv, envVar)
		}
	}
	updatedEnv = filteredEnv

	if r.coverDir != "" {
		// Coverage enabled: set GOCOVERDIR to our coverage directory
		updatedEnv = append(updatedEnv, fmt.Sprintf("GOCOVERDIR=%s", r.coverDir))
	}

	return updatedEnv
}

// BinaryPath returns the path to the binary being used.
func (r *AtmosRunner) BinaryPath() string {
	return r.binaryPath
}

// Cleanup removes temporary binary and its directory.
func (r *AtmosRunner) Cleanup() {
	if r.binaryPath != "" {
		tempDir := os.TempDir()
		binaryDir := filepath.Dir(r.binaryPath)
		// Clean paths for comparison.
		tempDir = filepath.Clean(tempDir)
		binaryDir = filepath.Clean(binaryDir)

		// Check if binary is in a temp subdirectory or directly in temp dir
		if binaryDir != tempDir && strings.HasPrefix(binaryDir, tempDir+string(filepath.Separator)) {
			// Binary is in a subdirectory of temp - remove the entire directory
			os.RemoveAll(binaryDir)
		} else if binaryDir == tempDir && (strings.Contains(filepath.Base(r.binaryPath), "atmos-test") || strings.Contains(filepath.Base(r.binaryPath), "cleanup-test")) {
			// Binary is directly in temp dir and looks like a test binary - remove just the file
			os.Remove(r.binaryPath)
		}
	}
}

// FindRepoRoot finds the root of the git repository.
func FindRepoRoot() (string, error) {
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
			return "", errUtils.ErrNotInGitRepository
		}
		dir = parent
	}
}
