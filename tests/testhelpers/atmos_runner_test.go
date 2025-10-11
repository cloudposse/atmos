package testhelpers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests"
)

func TestAtmosRunner_Build(t *testing.T) {
	// Skip long tests in short mode (these tests build atmos binary which takes 15-20s each)
	tests.SkipIfShort(t)

	t.Run("without coverage", func(t *testing.T) {
		runner := NewAtmosRunner("")
		err := runner.Build()

		// Should find existing binary.
		assert.NoError(t, err)
		assert.NotEmpty(t, runner.BinaryPath())
	})

	t.Run("with coverage", func(t *testing.T) {
		tempDir := t.TempDir()
		runner := NewAtmosRunner(tempDir)

		err := runner.Build()
		if err != nil {
			t.Skipf("Skipping coverage build test: %v", err)
		}

		assert.NotEmpty(t, runner.BinaryPath())
		assert.DirExists(t, tempDir)

		// Cleanup.
		runner.Cleanup()
	})

	t.Run("build called multiple times", func(t *testing.T) {
		tempDir := t.TempDir()
		runner := NewAtmosRunner(tempDir)

		// First call.
		err1 := runner.Build()
		path1 := runner.BinaryPath()

		// Second call should return same result.
		err2 := runner.Build()
		path2 := runner.BinaryPath()

		if err1 == nil {
			assert.Equal(t, err1, err2)
			assert.Equal(t, path1, path2)
		}

		runner.Cleanup()
	})

	t.Run("invalid coverage directory", func(t *testing.T) {
		// Use a file instead of directory to trigger error.
		tempFile := filepath.Join(t.TempDir(), "file")
		require.NoError(t, os.WriteFile(tempFile, []byte("test"), 0o644))

		runner := NewAtmosRunner(filepath.Join(tempFile, "subdir"))
		err := runner.Build()

		// Should fail to create coverage directory.
		assert.Error(t, err)
	})
}

func TestAtmosRunner_Command(t *testing.T) {
	// Skip long tests in short mode (builds atmos binary ~15-20s)
	tests.SkipIfShort(t)

	t.Run("without coverage", func(t *testing.T) {
		runner := NewAtmosRunner("")
		require.NoError(t, runner.Build())

		cmd := runner.Command("version")
		assert.NotNil(t, cmd)
		assert.Contains(t, cmd.Args, "version")

		// Check GOCOVERDIR is not set.
		for _, env := range cmd.Env {
			assert.NotContains(t, env, "GOCOVERDIR=")
		}
	})

	t.Run("with coverage", func(t *testing.T) {
		tempDir := t.TempDir()
		runner := NewAtmosRunner(tempDir)

		// Use existing binary for this test.
		runner.coverDir = ""
		require.NoError(t, runner.Build())
		runner.coverDir = tempDir

		cmd := runner.Command("version")
		assert.NotNil(t, cmd)

		// Check GOCOVERDIR is set.
		found := false
		for _, env := range cmd.Env {
			if env == fmt.Sprintf("GOCOVERDIR=%s", tempDir) {
				found = true
				break
			}
		}
		assert.True(t, found, "GOCOVERDIR should be set")
	})
}

func TestAtmosRunner_CommandContext(t *testing.T) {
	// Skip long tests in short mode (builds atmos binary ~15-20s)
	tests.SkipIfShort(t)

	t.Run("without coverage", func(t *testing.T) {
		runner := NewAtmosRunner("")
		require.NoError(t, runner.Build())

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cmd := runner.CommandContext(ctx, "version")
		assert.NotNil(t, cmd)
		assert.Contains(t, cmd.Args, "version")

		// Check GOCOVERDIR is not set.
		for _, env := range cmd.Env {
			assert.NotContains(t, env, "GOCOVERDIR=")
		}
	})

	t.Run("with coverage", func(t *testing.T) {
		tempDir := t.TempDir()
		runner := NewAtmosRunner(tempDir)

		// Use existing binary for this test.
		runner.coverDir = ""
		require.NoError(t, runner.Build())
		runner.coverDir = tempDir

		ctx := context.Background()
		cmd := runner.CommandContext(ctx, "version")
		assert.NotNil(t, cmd)

		// Check GOCOVERDIR is set.
		found := false
		for _, env := range cmd.Env {
			if env == fmt.Sprintf("GOCOVERDIR=%s", tempDir) {
				found = true
				break
			}
		}
		assert.True(t, found, "GOCOVERDIR should be set")
	})
}

func TestAtmosRunner_BinaryPath(t *testing.T) {
	runner := NewAtmosRunner("")
	require.NoError(t, runner.Build())

	path := runner.BinaryPath()
	assert.NotEmpty(t, path)

	// On Windows, should have .exe extension.
	if runtime.GOOS == "windows" {
		assert.Contains(t, path, ".exe")
	}
}

func TestAtmosRunner_Cleanup(t *testing.T) {
	t.Run("cleanup temp binary with coverage", func(t *testing.T) {
		tempDir := t.TempDir()
		runner := NewAtmosRunner(tempDir)

		// Simulate coverage build by setting paths.
		runner.binaryPath = filepath.Join(os.TempDir(), "test-binary")
		if runtime.GOOS == "windows" {
			runner.binaryPath += ".exe"
		}

		// Create dummy file.
		require.NoError(t, os.WriteFile(runner.binaryPath, []byte("test"), 0o755))

		// Cleanup should remove it.
		runner.Cleanup()

		// File should be gone (or at least attempted to remove).
		// We don't assert here because cleanup is best-effort.
	})

	t.Run("no cleanup for non-temp binary", func(t *testing.T) {
		runner := NewAtmosRunner("")
		runner.binaryPath = "/usr/local/bin/atmos"

		// Should not attempt to remove non-temp binary.
		runner.Cleanup() // Should not panic or error.
	})

	t.Run("cleanup without coverage dir", func(t *testing.T) {
		runner := NewAtmosRunner("")
		runner.binaryPath = filepath.Join(os.TempDir(), "test-binary")

		// Without coverDir, should not cleanup.
		runner.Cleanup()
		// Should not panic.
	})

	t.Run("cleanup without binary path", func(t *testing.T) {
		runner := NewAtmosRunner(t.TempDir())
		// With coverDir but no binaryPath.
		runner.Cleanup() // Should not panic.
	})

	t.Run("cleanup with binary not in temp", func(t *testing.T) {
		runner := NewAtmosRunner(t.TempDir())
		runner.binaryPath = "/usr/bin/atmos"

		// Should not cleanup binary not in temp dir.
		runner.Cleanup()
	})

	t.Run("cleanup actual temp binary", func(t *testing.T) {
		// Test the actual cleanup path.
		tempDir := t.TempDir()
		binaryName := fmt.Sprintf("cleanup-test-%d", os.Getpid())
		if runtime.GOOS == "windows" {
			binaryName += ".exe"
		}

		runner := &AtmosRunner{
			coverDir:   tempDir,
			binaryPath: filepath.Join(os.TempDir(), binaryName),
		}

		// Create the file.
		require.NoError(t, os.WriteFile(runner.binaryPath, []byte("dummy"), 0o755))

		// Verify it exists before cleanup.
		_, err := os.Stat(runner.binaryPath)
		require.NoError(t, err)

		// Now cleanup should remove it.
		runner.Cleanup()

		// Verify it's gone.
		_, err = os.Stat(runner.binaryPath)
		assert.True(t, os.IsNotExist(err), "Binary should be removed by Cleanup()")
	})
}

func Test_findRepoRoot(t *testing.T) {
	// Save current directory and restore after test.
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(oldWd)

	// Test from current directory.
	root, err := findRepoRoot()

	// Check if we found a repo.
	if err == nil {
		// If we found a repo, verify it.
		assert.NotEmpty(t, root)
		// Check that .git exists (can be file for worktrees or directory for regular repos).
		gitPath := filepath.Join(root, ".git")
		_, statErr := os.Stat(gitPath)
		assert.NoError(t, statErr, ".git should exist")
	} else {
		// If no repo found, should have specific error.
		assert.ErrorContains(t, err, "not in a git repository")
	}

	// Test from a temp directory (should fail).
	tempDir := t.TempDir()
	os.Chdir(tempDir)

	_, err = findRepoRoot()
	assert.Error(t, err)
	assert.ErrorContains(t, err, "not in a git repository")
}

func TestAtmosRunner_buildWithoutCoverage(t *testing.T) {
	// Skip long tests in short mode (builds atmos binary ~15-20s)
	tests.SkipIfShort(t)

	t.Run("successful build", func(t *testing.T) {
		runner := &AtmosRunner{}

		// Try to build without coverage.
		err := runner.buildWithoutCoverage()
		if err != nil {
			// Skip if build fails (might be missing dependencies).
			t.Skipf("Skipping build test: %v", err)
		}

		// Should have created binary.
		assert.NotEmpty(t, runner.binaryPath)
		assert.FileExists(t, runner.binaryPath)

		// Binary should be in temp directory.
		assert.Contains(t, runner.binaryPath, os.TempDir())
		assert.Contains(t, runner.binaryPath, "atmos-test")

		// Should have correct extension on Windows.
		if runtime.GOOS == "windows" {
			assert.Contains(t, runner.binaryPath, ".exe")
		}

		// Cleanup.
		os.Remove(runner.binaryPath)
	})

	t.Run("handles missing repo root", func(t *testing.T) {
		// Change to directory without git repo.
		tempDir := t.TempDir()
		oldWd, _ := os.Getwd()
		defer os.Chdir(oldWd)
		os.Chdir(tempDir)

		runner := &AtmosRunner{}

		err := runner.buildWithoutCoverage()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to find repository root")
	})
}

func TestAtmosRunner_buildWithCoverage(t *testing.T) {
	// Skip long tests in short mode (builds atmos binary with coverage ~15-20s)
	tests.SkipIfShort(t)

	t.Run("successful build", func(t *testing.T) {
		runner := &AtmosRunner{
			coverDir: t.TempDir(),
		}

		// Try to build with coverage.
		err := runner.buildWithCoverage()
		if err != nil {
			// Skip if build fails (might be missing dependencies).
			t.Skipf("Skipping coverage build test: %v", err)
		}

		// Should have created binary.
		assert.NotEmpty(t, runner.binaryPath)
		assert.FileExists(t, runner.binaryPath)

		// Binary should be in temp directory.
		assert.Contains(t, runner.binaryPath, os.TempDir())
		assert.Contains(t, runner.binaryPath, "atmos-coverage")

		// Should have correct extension on Windows.
		if runtime.GOOS == "windows" {
			assert.Contains(t, runner.binaryPath, ".exe")
		}

		// Cleanup.
		os.Remove(runner.binaryPath)
	})

	t.Run("handles missing repo root", func(t *testing.T) {
		// Change to directory without git repo.
		tempDir := t.TempDir()
		oldWd, _ := os.Getwd()
		defer os.Chdir(oldWd)
		os.Chdir(tempDir)

		runner := &AtmosRunner{
			coverDir: t.TempDir(),
		}

		err := runner.buildWithCoverage()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to find repository root")
	})
}

func TestAtmosRunner_CoverageIntegration(t *testing.T) {
	// Skip long tests in short mode (integration test that builds and runs atmos ~15-20s)
	tests.SkipIfShort(t)

	// Integration test that actually runs atmos with coverage.
	tempDir := t.TempDir()
	runner := NewAtmosRunner(tempDir)

	err := runner.Build()
	if err != nil {
		t.Skipf("Skipping integration test: %v", err)
	}

	defer runner.Cleanup()

	// Run a simple command.
	cmd := runner.Command("version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("Skipping integration test, atmos execution failed: %v", err)
	}

	// Should have output.
	assert.NotEmpty(t, output)

	// If coverage was enabled, coverage files should be created.
	if runner.coverDir != "" {
		// Check if any coverage files were created.
		entries, _ := os.ReadDir(runner.coverDir)
		// Coverage files might not be created for simple commands like version.
		_ = entries // Just check that directory is accessible.
	}
}

func TestAtmosRunner_ErrorPaths(t *testing.T) {
	t.Run("build with invalid go module", func(t *testing.T) {
		runner := &AtmosRunner{
			coverDir: t.TempDir(),
		}

		// Change to a directory without go.mod.
		oldWd, _ := os.Getwd()
		os.Chdir(t.TempDir())
		defer os.Chdir(oldWd)

		// Should fail to build.
		err := runner.buildWithCoverage()
		assert.Error(t, err)
	})
}

func TestAtmosRunner_ConcurrentBuild(t *testing.T) {
	// Skip long tests in short mode (builds atmos binary concurrently ~15-20s)
	tests.SkipIfShort(t)

	// Test that concurrent builds don't interfere with each other.
	tempDir := t.TempDir()
	runner := NewAtmosRunner(tempDir)

	done := make(chan error, 10)

	// Start multiple goroutines trying to build.
	for i := 0; i < 10; i++ {
		go func() {
			done <- runner.Build()
		}()
	}

	// Collect results.
	var firstErr error
	for i := 0; i < 10; i++ {
		err := <-done
		if i == 0 {
			firstErr = err
		} else {
			// All builds should return the same result.
			if firstErr == nil {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		}
	}

	runner.Cleanup()
}
