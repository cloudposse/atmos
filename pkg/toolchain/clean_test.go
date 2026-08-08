package toolchain

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	// Permission mask for created directories in tests (defaultMkdirPermissions).
	defaultMkdirPermissions = 0o755
)

// verifyPermissionErrorOutput verifies output when permission errors occur.
func verifyPermissionErrorOutput(t *testing.T, output, expectedOutput, cacheDir string) {
	t.Helper()

	outputLines := strings.Split(output, "\n")
	expectedLines := strings.Split(expectedOutput, "\n")
	if len(outputLines) != len(expectedLines) {
		t.Errorf("unexpected number of output lines:\nGot: %d\nWant: %d\nGot:\n%s\nWant:\n%s", len(outputLines), len(expectedLines), output, expectedOutput)
	}
	for i, expectedLine := range expectedLines {
		if i >= len(outputLines) {
			continue
		}
		if strings.Contains(expectedLine, "Warning") {
			verifyWarningLine(t, outputLines[i], expectedLine, cacheDir)
		} else if outputLines[i] != expectedLine {
			t.Errorf("unexpected output line:\nGot: %s\nWant: %s", outputLines[i], expectedLine)
		}
	}
}

// verifyWarningLine checks that a warning line contains the expected directory and error.
func verifyWarningLine(t *testing.T, actual, expected, cacheDir string) {
	t.Helper()

	// Flexible check for warnings: must contain directory and "permission denied".
	if !strings.Contains(actual, cacheDir) || !strings.Contains(actual, "permission denied") {
		t.Errorf("unexpected warning line:\nGot: %s\nWant containing: %s", actual, expected)
	}
}

// createCleanTestDir creates a test directory with files and subdirectories.
func createCleanTestDir(t *testing.T, baseDir string, files, dirs []string) {
	t.Helper()

	if err := os.MkdirAll(baseDir, defaultMkdirPermissions); err != nil {
		t.Fatalf("failed to create test dir %s: %v", baseDir, err)
	}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(baseDir, file), []byte("test"), defaultFileWritePermissions); err != nil {
			t.Fatalf("failed to create file %s: %v", file, err)
		}
	}
	for _, dir := range dirs {
		if err := os.Mkdir(filepath.Join(baseDir, dir), defaultMkdirPermissions); err != nil {
			t.Fatalf("failed to create dir %s: %v", dir, err)
		}
	}
}

// resetCleanTestPermissions resets permissions for cleanup.
func resetCleanTestPermissions(t *testing.T, dir string) {
	t.Helper()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return
	}
	// First, set dir to 0700 to allow traversal and modification for owner.
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Logf("failed to set dir writable for reset: %v", err)
		return
	}
	// Then recursively reset permissions.
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chmod(path, defaultMkdirPermissions)
	})
	if err != nil {
		t.Logf("failed to reset permissions for %s: %v", dir, err)
	}
}

// captureCleanTestOutput captures stderr output for testing.
func captureCleanTestOutput(t *testing.T, f func()) string {
	t.Helper()

	originalStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}

	// Redirect stderr to our pipe.
	os.Stderr = w

	// Set NO_COLOR to disable ANSI codes in output for deterministic testing.
	// Also set force-tty with wide terminal width (120 columns) to prevent word wrapping.
	// Without this, the terminal width detection fails on pipes and causes
	// unpredictable line wrapping in the output.
	t.Setenv("NO_COLOR", "1")
	viper.Set("force-tty", true)
	t.Cleanup(func() {
		viper.Set("force-tty", false)
	})

	// Initialize IO context for UI functions.
	ioCtx, err := iolib.NewContext()
	if err != nil {
		t.Fatalf("failed to create IO context: %v", err)
	}
	ui.InitFormatter(ioCtx)

	// Ensure stderr is restored and read pipe is closed even if f() panics.
	defer func() {
		os.Stderr = originalStderr
	}()
	defer r.Close()

	f()

	// Close write end to unblock ReadFrom.
	w.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("failed to read from pipe: %v", err)
	}

	return buf.String()
}

// captureUITestOutput redirects the ui package's output to an in-memory buffer for the duration
// of f, without forcing TTY mode or touching the real os.Stderr file descriptor. Unlike
// captureCleanTestOutput's os.Pipe()-based redirect (which is only drained after f returns), a
// bytes.Buffer never blocks on write -- required for any test that exercises a live,
// ticker-driven renderer (see batch_progress.go's liveBatchRenderer): with force-tty forcing the
// renderer active, repeated writes into an os.Pipe that's never concurrently drained can fill the
// pipe's bounded OS buffer and block forever once the batch takes more than a few seconds,
// deadlocking the test (observed hanging a full 40 minutes on Windows CI before Go's own test
// timeout killed it). Use this helper instead of captureCleanTestOutput for any test that runs a
// real concurrent RunUpdate/RunLock batch and only needs to assert on output content, not exact
// non-wrapped column widths.
func captureUITestOutput(t *testing.T, f func()) string {
	t.Helper()

	// Defensively force TTY detection off regardless of global viper state another test may
	// have left behind (force-tty is a process-wide setting) -- the live batch renderer must
	// never activate against this buffer-backed, unread-until-return stream.
	previousForceTTY := viper.GetBool("force-tty")
	viper.Set("force-tty", false)
	t.Cleanup(func() { viper.Set("force-tty", previousForceTTY) })

	var buf bytes.Buffer
	streams := &testStreams{stdin: &bytes.Buffer{}, stdout: &bytes.Buffer{}, stderr: &buf}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	if err != nil {
		t.Fatalf("failed to create IO context: %v", err)
	}
	ui.InitFormatter(ioCtx)

	f()

	return buf.String()
}

// cleanTestCase represents a test case for CleanToolsAndCaches.
type cleanTestCase struct {
	name           string
	setup          func(t *testing.T) (toolsDir, cacheDir, tempCacheDir string)
	expectedError  bool
	expectedOutput string
}

// getCleanTestCases returns the test cases for CleanToolsAndCaches.
func getCleanTestCases() []cleanTestCase {
	return []cleanTestCase{
		{
			name: "HappyPath_AllDirectoriesWithContent",
			setup: func(t *testing.T) (string, string, string) {
				base := t.TempDir()
				toolsDir := filepath.Join(base, "tools")
				cacheDir := filepath.Join(base, "cache")
				tempCacheDir := filepath.Join(base, "temp-cache")
				createCleanTestDir(t, toolsDir, []string{"file1", "file2"}, []string{"dir1"})
				createCleanTestDir(t, cacheDir, []string{"cache1"}, []string{"cachedir1"})
				createCleanTestDir(t, tempCacheDir, []string{"temp1"}, []string{})
				return toolsDir, cacheDir, tempCacheDir
			},
			expectedError: false,
			// In NO_COLOR mode, markdown is rendered to plain text (bold becomes plain).
			expectedOutput: `✓ Deleted 3 files/directories from %s
✓ Deleted 2 files from %s cache
✓ Deleted 1 files from %s cache
`,
		},
		{
			name: "NonExistentDirectories",
			setup: func(t *testing.T) (string, string, string) {
				base := t.TempDir()
				return filepath.Join(base, "nonexistent-tools"),
					filepath.Join(base, "nonexistent-cache"),
					filepath.Join(base, "nonexistent-temp-cache")
			},
			expectedError: false,
			// In NO_COLOR mode, markdown is rendered to plain text.
			expectedOutput: `✓ Deleted 0 files/directories from %s
`,
		},
		{
			name: "EmptyDirectories",
			setup: func(t *testing.T) (string, string, string) {
				base := t.TempDir()
				toolsDir := filepath.Join(base, "tools")
				cacheDir := filepath.Join(base, "cache")
				tempCacheDir := filepath.Join(base, "temp-cache")
				if err := os.MkdirAll(toolsDir, defaultMkdirPermissions); err != nil {
					t.Fatalf("failed to create toolsDir: %v", err)
				}
				if err := os.MkdirAll(cacheDir, defaultMkdirPermissions); err != nil {
					t.Fatalf("failed to create cacheDir: %v", err)
				}
				if err := os.MkdirAll(tempCacheDir, defaultMkdirPermissions); err != nil {
					t.Fatalf("failed to create tempCacheDir: %v", err)
				}
				return toolsDir, cacheDir, tempCacheDir
			},
			expectedError: false,
			// In NO_COLOR mode, markdown is rendered to plain text.
			expectedOutput: `✓ Deleted 0 files/directories from %s
`,
		},
		newPermissionErrorToolsDirTestCase(),
		newPermissionErrorCacheDirTestCase(),
	}
}

// newPermissionErrorToolsDirTestCase creates a test case for tools dir permission errors.
func newPermissionErrorToolsDirTestCase() cleanTestCase {
	return cleanTestCase{
		name: "PermissionError_ToolsDir",
		setup: func(t *testing.T) (string, string, string) {
			if runtime.GOOS == "windows" {
				t.Skip()
				return "", "", ""
			}
			base := t.TempDir()
			toolsDir := filepath.Join(base, "tools")
			cacheDir := filepath.Join(base, "cache")
			tempCacheDir := filepath.Join(base, "temp-cache")
			createCleanTestDir(t, toolsDir, []string{"file1"}, []string{})
			// Set file read-only FIRST.
			if err := os.Chmod(filepath.Join(toolsDir, "file1"), 0o400); err != nil {
				t.Fatalf("failed to set file permissions: %v", err)
			}
			// Then set directory read-only (no execute).
			if err := os.Chmod(toolsDir, 0o400); err != nil {
				t.Fatalf("failed to set directory permissions: %v", err)
			}
			createCleanTestDir(t, cacheDir, []string{"cache1"}, []string{})
			createCleanTestDir(t, tempCacheDir, []string{"temp1"}, []string{})
			// Defer permission reset for cleanup.
			t.Cleanup(func() {
				resetCleanTestPermissions(t, toolsDir)
			})
			return toolsDir, cacheDir, tempCacheDir
		},
		expectedError:  true,
		expectedOutput: "",
	}
}

// newPermissionErrorCacheDirTestCase creates a test case for cache dir permission errors.
func newPermissionErrorCacheDirTestCase() cleanTestCase {
	return cleanTestCase{
		name: "PermissionError_CacheDir",
		setup: func(t *testing.T) (string, string, string) {
			if runtime.GOOS == "windows" {
				t.Skip()
				return "", "", ""
			}
			base := t.TempDir()
			toolsDir := filepath.Join(base, "tools")
			cacheDir := filepath.Join(base, "cache")
			tempCacheDir := filepath.Join(base, "temp-cache")
			createCleanTestDir(t, toolsDir, []string{"file1"}, []string{})
			createCleanTestDir(t, cacheDir, []string{"cache1"}, []string{})
			// Set file read-only FIRST.
			if err := os.Chmod(filepath.Join(cacheDir, "cache1"), 0o400); err != nil {
				t.Fatalf("failed to set file permissions: %v", err)
			}
			// Then set directory read-only (no execute).
			if err := os.Chmod(cacheDir, 0o400); err != nil {
				t.Fatalf("failed to set directory permissions: %v", err)
			}
			createCleanTestDir(t, tempCacheDir, []string{"temp1"}, []string{})
			// Defer permission reset for cleanup.
			t.Cleanup(func() {
				resetCleanTestPermissions(t, cacheDir)
			})
			return toolsDir, cacheDir, tempCacheDir
		},
		expectedError: false,
		// In NO_COLOR mode, markdown is rendered to plain text.
		expectedOutput: `Warning: failed to count files in %s: permission denied
Warning: failed to delete %s: permission denied
✓ Deleted 1 files/directories from %s
✓ Deleted 1 files from %s cache
`,
	}
}

func TestCleanToolsAndCaches(t *testing.T) {
	tests := getCleanTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runCleanTestCase(t, tt)
		})
	}
}

// runCleanTestCase executes a single test case.
func runCleanTestCase(t *testing.T, tt cleanTestCase) {
	t.Helper()

	toolsDir, cacheDir, tempCacheDir := tt.setup(t)

	// Capture output.
	output := captureCleanTestOutput(t, func() {
		err := CleanToolsAndCaches(toolsDir, cacheDir, tempCacheDir)
		if (err != nil) != tt.expectedError {
			t.Errorf("expected error: %v, got: %v", tt.expectedError, err)
		}
	})

	expectedOutput := formatExpectedOutput(tt.expectedOutput, toolsDir, cacheDir, tempCacheDir)
	output = normalizeTestOutput(output)
	expectedOutput = normalizeTestOutput(expectedOutput)

	verifyCleanTestOutput(t, tt.name, output, expectedOutput, cacheDir)
	verifyCleanTestDirectories(t, tt, toolsDir, cacheDir, tempCacheDir)
}

// formatExpectedOutput formats the expected output based on the number of placeholders.
func formatExpectedOutput(template, toolsDir, cacheDir, tempCacheDir string) string {
	if template == "" {
		return ""
	}
	switch {
	case strings.Contains(template, "Warning"):
		// For permission errors, use simplified placeholder (flexible comparison handles details).
		return fmt.Sprintf(template, cacheDir, cacheDir, toolsDir, tempCacheDir)
	case strings.Count(template, "%s") == 1:
		return fmt.Sprintf(template, toolsDir)
	default:
		return fmt.Sprintf(template, toolsDir, cacheDir, tempCacheDir)
	}
}

// normalizeTestOutput normalizes output for cross-platform comparison.
func normalizeTestOutput(output string) string {
	// Normalize line endings for cross-platform compatibility.
	output = strings.ReplaceAll(output, "\r\n", "\n")

	// Normalize word-wrapped continuation lines.
	// The markdown renderer wraps long lines with "\n  " (newline + 2 space indent).
	// Collapse these back to single-line format for deterministic comparison.
	output = strings.ReplaceAll(output, "\n  ", " ")

	// Strip trailing whitespace from each line.
	// Glamour adds padding spaces to fill terminal width during word wrapping.
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	output = strings.Join(lines, "\n")

	// Fix hyphen word breaks from markdown wrapping.
	// Pattern: hyphen followed by space and lowercase letter (indicates mid-word break).
	for _, segment := range []string{"temp- cache", "nonexistent- tools"} {
		fixed := strings.ReplaceAll(segment, "- ", "-")
		output = strings.ReplaceAll(output, segment, fixed)
	}

	return output
}

// verifyCleanTestOutput verifies the test output matches expected.
func verifyCleanTestOutput(t *testing.T, testName, output, expectedOutput, cacheDir string) {
	t.Helper()

	if strings.Contains(testName, "PermissionError_CacheDir") {
		verifyPermissionErrorOutput(t, output, expectedOutput, cacheDir)
	} else if output != expectedOutput {
		t.Errorf("unexpected output:\nGot:\n%s\nWant:\n%s", output, expectedOutput)
	}
}

// verifyCleanTestDirectories verifies directories were deleted (or not, in case of errors).
func verifyCleanTestDirectories(t *testing.T, tt cleanTestCase, toolsDir, cacheDir, tempCacheDir string) {
	t.Helper()

	if tt.expectedError {
		return
	}

	if _, err := os.Stat(toolsDir); !os.IsNotExist(err) {
		t.Errorf("toolsDir %s should be deleted", toolsDir)
	}
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) && !strings.Contains(tt.name, "PermissionError_CacheDir") {
		t.Errorf("cacheDir %s should be deleted", cacheDir)
	}
	if _, err := os.Stat(tempCacheDir); !os.IsNotExist(err) {
		t.Errorf("tempCacheDir %s should be deleted", tempCacheDir)
	}
}

// setUpRunCleanFixture creates toolsDir/cacheDir/tempCacheDir, each with one file, for RunClean tests.
func setUpRunCleanFixture(t *testing.T) (toolsDir, cacheDir, tempCacheDir string) {
	t.Helper()

	base := t.TempDir()
	toolsDir = filepath.Join(base, "tools")
	cacheDir = filepath.Join(base, "cache")
	tempCacheDir = filepath.Join(base, "temp-cache")
	createCleanTestDir(t, toolsDir, []string{"terraform"}, []string{})
	createCleanTestDir(t, cacheDir, []string{"download.tar.gz"}, []string{})
	createCleanTestDir(t, tempCacheDir, []string{"extract.tmp"}, []string{})
	return toolsDir, cacheDir, tempCacheDir
}

// assertDirUnchanged asserts that dir still exists and still contains exactly the given entries.
func assertDirUnchanged(t *testing.T, dir string, wantEntries []string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "directory %s should still exist", dir)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.ElementsMatch(t, wantEntries, names, "directory %s contents should be unchanged", dir)
}

func TestRunClean_DryRun_DeletesNothing(t *testing.T) {
	toolsDir, cacheDir, tempCacheDir := setUpRunCleanFixture(t)

	var output string
	require.NotPanics(t, func() {
		output = captureCleanTestOutput(t, func() {
			err := RunClean(toolsDir, cacheDir, tempCacheDir, CleanOptions{DryRun: true})
			require.NoError(t, err)
		})
	})

	// Nothing should have been deleted.
	assertDirUnchanged(t, toolsDir, []string{"terraform"})
	assertDirUnchanged(t, cacheDir, []string{"download.tar.gz"})
	assertDirUnchanged(t, tempCacheDir, []string{"extract.tmp"})

	assert.Contains(t, output, "Would delete")
	assert.Contains(t, output, toolsDir)
	assert.Contains(t, output, cacheDir)
	assert.Contains(t, output, tempCacheDir)
}

func TestRunClean_DryRun_CacheOnly_SkipsToolsDir(t *testing.T) {
	toolsDir, cacheDir, tempCacheDir := setUpRunCleanFixture(t)

	output := captureCleanTestOutput(t, func() {
		err := RunClean(toolsDir, cacheDir, tempCacheDir, CleanOptions{DryRun: true, CacheOnly: true})
		require.NoError(t, err)
	})

	// Nothing deleted, and the tools dir isn't even mentioned since --cache-only skips it.
	assertDirUnchanged(t, toolsDir, []string{"terraform"})
	assertDirUnchanged(t, cacheDir, []string{"download.tar.gz"})
	assertDirUnchanged(t, tempCacheDir, []string{"extract.tmp"})
	assert.NotContains(t, output, toolsDir)
	assert.Contains(t, output, cacheDir)
}

func TestRunClean_CacheOnly_LeavesInstalledToolsIntact(t *testing.T) {
	toolsDir, cacheDir, tempCacheDir := setUpRunCleanFixture(t)

	captureCleanTestOutput(t, func() {
		err := RunClean(toolsDir, cacheDir, tempCacheDir, CleanOptions{CacheOnly: true, Force: true})
		require.NoError(t, err)
	})

	// Tools dir must survive --cache-only; cache dirs must be gone.
	assertDirUnchanged(t, toolsDir, []string{"terraform"})
	_, err := os.Stat(cacheDir)
	assert.True(t, os.IsNotExist(err), "cacheDir should be deleted")
	_, err = os.Stat(tempCacheDir)
	assert.True(t, os.IsNotExist(err), "tempCacheDir should be deleted")
}

func TestRunClean_Force_DeletesEverything(t *testing.T) {
	toolsDir, cacheDir, tempCacheDir := setUpRunCleanFixture(t)

	captureCleanTestOutput(t, func() {
		err := RunClean(toolsDir, cacheDir, tempCacheDir, CleanOptions{Force: true})
		require.NoError(t, err)
	})

	for _, dir := range []string{toolsDir, cacheDir, tempCacheDir} {
		_, err := os.Stat(dir)
		assert.True(t, os.IsNotExist(err), "%s should be deleted", dir)
	}
}

// TestRunClean_NonInteractive_RequiresForce verifies that, without --force and without a TTY,
// RunClean refuses to delete anything and reports ErrToolchainCleanRequiresConfirmation
// instead of silently proceeding or hanging on a prompt. It forces the non-interactive branch
// via the isTTYForStdoutFunc seam rather than depending on whether the test runner itself has a
// TTY attached (which varies: none in CI, but a real /dev/tty when run manually in a terminal —
// and the real huh prompt requires an actual /dev/tty it can open, which isn't available here).
func TestRunClean_NonInteractive_RequiresForce(t *testing.T) {
	toolsDir, cacheDir, tempCacheDir := setUpRunCleanFixture(t)

	originalIsTTY := isTTYForStdoutFunc
	isTTYForStdoutFunc = func() bool { return false }
	t.Cleanup(func() { isTTYForStdoutFunc = originalIsTTY })

	var err error
	captureCleanTestOutput(t, func() {
		err = RunClean(toolsDir, cacheDir, tempCacheDir, CleanOptions{})
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrToolchainCleanRequiresConfirmation)

	// Nothing should have been deleted.
	assertDirUnchanged(t, toolsDir, []string{"terraform"})
	assertDirUnchanged(t, cacheDir, []string{"download.tar.gz"})
	assertDirUnchanged(t, tempCacheDir, []string{"extract.tmp"})
}

func TestRunClean_DryRun_NonExistentDirs(t *testing.T) {
	base := t.TempDir()
	toolsDir := filepath.Join(base, "missing-tools")
	cacheDir := filepath.Join(base, "missing-cache")
	tempCacheDir := filepath.Join(base, "missing-temp-cache")

	output := captureCleanTestOutput(t, func() {
		err := RunClean(toolsDir, cacheDir, tempCacheDir, CleanOptions{DryRun: true})
		require.NoError(t, err)
	})

	assert.Contains(t, output, "does not exist")
}
