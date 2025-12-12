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

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
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
			// Flexible check for warnings: must contain directory and "permission denied".
			if !strings.Contains(outputLines[i], cacheDir) || !strings.Contains(outputLines[i], "permission denied") {
				t.Errorf("unexpected warning line:\nGot: %s\nWant containing: %s", outputLines[i], expectedLine)
			}
		} else if outputLines[i] != expectedLine {
			t.Errorf("unexpected output line:\nGot: %s\nWant: %s", outputLines[i], expectedLine)
		}
	}
}

func TestCleanToolsAndCaches(t *testing.T) {
	// Helper to create a test directory with files and subdirectories
	createTestDir := func(t *testing.T, baseDir string, files, dirs []string) {
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

	// Helper to reset permissions for cleanup
	resetPermissions := func(t *testing.T, dir string) {
		t.Helper()
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return
		}
		// First, set dir to 0700 to allow traversal and modification for owner
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Logf("failed to set dir writable for reset: %v", err)
			return
		}
		// Then recursively reset permissions
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

	// Helper to capture stderr (TUI output goes to stderr).
	captureOutput := func(t *testing.T, f func()) string {
		originalStderr := os.Stderr
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("failed to create pipe: %v", err)
		}

		// Redirect stderr to our pipe
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

		// Initialize IO context for UI functions
		ioCtx, err := iolib.NewContext()
		if err != nil {
			t.Fatalf("failed to create IO context: %v", err)
		}
		ui.InitFormatter(ioCtx)

		// Ensure stderr is restored and pipes are closed even if f() panics.
		defer func() {
			os.Stderr = originalStderr
		}()
		defer r.Close()
		defer w.Close()

		f()

		// Close write end to unblock ReadFrom.
		w.Close()

		var buf bytes.Buffer
		if _, err := buf.ReadFrom(r); err != nil {
			t.Fatalf("failed to read from pipe: %v", err)
		}

		return buf.String()
	}

	// Test cases
	tests := []struct {
		name           string
		setup          func(t *testing.T) (toolsDir, cacheDir, tempCacheDir string)
		expectedError  bool
		expectedOutput string
	}{
		{
			name: "HappyPath_AllDirectoriesWithContent",
			setup: func(t *testing.T) (string, string, string) {
				base := t.TempDir()
				toolsDir := filepath.Join(base, "tools")
				cacheDir := filepath.Join(base, "cache")
				tempCacheDir := filepath.Join(base, "temp-cache")
				createTestDir(t, toolsDir, []string{"file1", "file2"}, []string{"dir1"})
				createTestDir(t, cacheDir, []string{"cache1"}, []string{"cachedir1"})
				createTestDir(t, tempCacheDir, []string{"temp1"}, []string{})
				return toolsDir, cacheDir, tempCacheDir
			},
			expectedError: false,
			expectedOutput: `✓ Deleted **3** files/directories from %s
✓ Deleted **2** files from %s cache
✓ Deleted **1** files from %s cache
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
			expectedOutput: `✓ Deleted **0** files/directories from %s
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
			expectedOutput: `✓ Deleted **0** files/directories from %s
`,
		},
		{
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
				createTestDir(t, toolsDir, []string{"file1"}, []string{})
				// Set file read-only FIRST
				if err := os.Chmod(filepath.Join(toolsDir, "file1"), 0o400); err != nil {
					t.Fatalf("failed to set file permissions: %v", err)
				}
				// Then set directory read-only (no execute)
				if err := os.Chmod(toolsDir, 0o400); err != nil {
					t.Fatalf("failed to set directory permissions: %v", err)
				}
				createTestDir(t, cacheDir, []string{"cache1"}, []string{})
				createTestDir(t, tempCacheDir, []string{"temp1"}, []string{})
				// Defer permission reset for cleanup
				t.Cleanup(func() {
					resetPermissions(t, toolsDir)
				})
				return toolsDir, cacheDir, tempCacheDir
			},
			expectedError:  true,
			expectedOutput: "",
		},
		{
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
				createTestDir(t, toolsDir, []string{"file1"}, []string{})
				createTestDir(t, cacheDir, []string{"cache1"}, []string{})
				// Set file read-only FIRST
				if err := os.Chmod(filepath.Join(cacheDir, "cache1"), 0o400); err != nil {
					t.Fatalf("failed to set file permissions: %v", err)
				}
				// Then set directory read-only (no execute)
				if err := os.Chmod(cacheDir, 0o400); err != nil {
					t.Fatalf("failed to set directory permissions: %v", err)
				}
				createTestDir(t, tempCacheDir, []string{"temp1"}, []string{})
				// Defer permission reset for cleanup
				t.Cleanup(func() {
					resetPermissions(t, cacheDir)
				})
				return toolsDir, cacheDir, tempCacheDir
			},
			expectedError: false,
			expectedOutput: `Warning: failed to count files in %s: permission denied
Warning: failed to delete %s: permission denied
✓ Deleted **1** files/directories from %s
✓ Deleted **1** files from %s cache
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolsDir, cacheDir, tempCacheDir := tt.setup(t)

			// Capture output
			output := captureOutput(t, func() {
				err := CleanToolsAndCaches(toolsDir, cacheDir, tempCacheDir)
				if (err != nil) != tt.expectedError {
					t.Errorf("expected error: %v, got: %v", tt.expectedError, err)
				}
			})

			// Format expected output based on the number of placeholders
			var expectedOutput string
			if tt.expectedOutput != "" {
				switch {
				case strings.Contains(tt.expectedOutput, "Warning"):
					// For permission errors, use simplified placeholder (flexible comparison handles details)
					expectedOutput = fmt.Sprintf(tt.expectedOutput, cacheDir, cacheDir, toolsDir, tempCacheDir)
				case strings.Count(tt.expectedOutput, "%s") == 1:
					expectedOutput = fmt.Sprintf(tt.expectedOutput, toolsDir)
				default:
					expectedOutput = fmt.Sprintf(tt.expectedOutput, toolsDir, cacheDir, tempCacheDir)
				}
			}

			// Normalize line endings for cross-platform compatibility
			output = strings.ReplaceAll(output, "\r\n", "\n")
			expectedOutput = strings.ReplaceAll(expectedOutput, "\r\n", "\n")

			// Normalize word-wrapped continuation lines.
			// The markdown renderer wraps long lines with "\n  " (newline + 2 space indent).
			// Collapse these back to single-line format for deterministic comparison.
			// Also handle hyphen breaks where "word-\n  word" becomes "word- word".
			// We need to rejoin these without the space (e.g., "temp- cache" -> "temp-cache").
			output = strings.ReplaceAll(output, "\n  ", " ")

			// Fix hyphen word breaks from markdown wrapping.
			// Pattern: hyphen followed by space and lowercase letter (indicates mid-word break).
			// Only fix in path context (not "temp-cache cache" which is correct).
			// The pattern "- " followed by a word that would form a valid path segment.
			for _, segment := range []string{"temp- cache", "nonexistent- tools"} {
				fixed := strings.ReplaceAll(segment, "- ", "-")
				output = strings.ReplaceAll(output, segment, fixed)
			}

			// Verify output
			if strings.Contains(tt.name, "PermissionError_CacheDir") {
				verifyPermissionErrorOutput(t, output, expectedOutput, cacheDir)
			} else if output != expectedOutput {
				t.Errorf("unexpected output:\nGot:\n%s\nWant:\n%s", output, expectedOutput)
			}

			// Verify directories were deleted (or not, in case of errors)
			if !tt.expectedError {
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
		})
	}
}
