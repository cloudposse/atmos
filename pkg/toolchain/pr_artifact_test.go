package toolchain

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/github"
)

// Note: IsPRVersion tests are in version_spec_test.go.

func TestSanitizeZipPath(t *testing.T) {
	// Use t.TempDir() to get a platform-appropriate temp directory.
	baseDir := t.TempDir()
	destDir := filepath.Clean(baseDir) + string(os.PathSeparator)

	tests := []struct {
		name      string
		entryName string
		wantPath  string
		wantErr   bool
	}{
		// Valid paths.
		{
			name:      "simple file",
			entryName: "atmos",
			wantPath:  filepath.Join(baseDir, "atmos"),
			wantErr:   false,
		},
		{
			name:      "file in subdirectory",
			entryName: "build/atmos",
			wantPath:  filepath.Join(baseDir, "build", "atmos"),
			wantErr:   false,
		},
		{
			name:      "nested subdirectory",
			entryName: "a/b/c/file.txt",
			wantPath:  filepath.Join(baseDir, "a", "b", "c", "file.txt"),
			wantErr:   false,
		},

		// Zip Slip attack patterns (should error).
		{
			name:      "simple path traversal",
			entryName: "../etc/passwd",
			wantErr:   true,
		},
		{
			name:      "deep path traversal",
			entryName: "../../../etc/passwd",
			wantErr:   true,
		},
		{
			name:      "hidden path traversal",
			entryName: "build/../../../etc/passwd",
			wantErr:   true,
		},
		{
			name:      "windows-style traversal",
			entryName: "..\\etc\\passwd",
			wantErr:   true,
		},
		{
			name:      "absolute path unix",
			entryName: "/etc/passwd",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, err := sanitizeZipPath(tt.entryName, destDir)

			if tt.wantErr {
				require.Error(t, err, "expected error for malicious path")
				assert.True(t, errors.Is(err, ErrPRArtifactExtractFailed),
					"error should wrap ErrPRArtifactExtractFailed")
				assert.Contains(t, err.Error(), "Zip Slip")
				return
			}

			require.NoError(t, err, "unexpected error")
			assert.Equal(t, tt.wantPath, gotPath, "path mismatch")
		})
	}
}

func TestBuildTokenRequiredError(t *testing.T) {
	err := buildTokenRequiredError()
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
}

func TestHandlePRArtifactError(t *testing.T) {
	t.Run("generic error returns tool installation error", func(t *testing.T) {
		err := handlePRArtifactError(assert.AnError, 2038)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrToolInstall)
	})
}

func TestHandlePRArtifactError_AllCases(t *testing.T) {
	tests := []struct {
		name         string
		inputErr     error
		prNumber     int
		expectedType error
	}{
		{
			name:         "PR not found",
			inputErr:     fmt.Errorf("wrapped: %w", github.ErrPRNotFound),
			prNumber:     999,
			expectedType: errUtils.ErrToolNotFound,
		},
		{
			name:         "no workflow run found",
			inputErr:     fmt.Errorf("wrapped: %w", github.ErrNoWorkflowRunFound),
			prNumber:     999,
			expectedType: errUtils.ErrToolNotFound,
		},
		{
			name:         "no artifact found",
			inputErr:     fmt.Errorf("wrapped: %w", github.ErrNoArtifactFound),
			prNumber:     999,
			expectedType: errUtils.ErrToolNotFound,
		},
		{
			name:         "no artifact for platform",
			inputErr:     fmt.Errorf("wrapped: %w", github.ErrNoArtifactForPlatform),
			prNumber:     999,
			expectedType: errUtils.ErrToolPlatformNotSupported,
		},
		{
			name:         "generic error",
			inputErr:     assert.AnError,
			prNumber:     999,
			expectedType: errUtils.ErrToolInstall,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handlePRArtifactError(tt.inputErr, tt.prNumber)
			assert.Error(t, err)
			assert.ErrorIs(t, err, tt.expectedType)
		})
	}
}

// createTestZip creates a ZIP file at zipPath containing the given file entries.
// Each entry is a map of file path -> content.
func createTestZip(t *testing.T, zipPath string, entries map[string][]byte) {
	t.Helper()

	f, err := os.Create(zipPath)
	require.NoError(t, err)
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	for name, content := range entries {
		fw, err := w.Create(name)
		require.NoError(t, err)
		_, err = fw.Write(content)
		require.NoError(t, err)
	}
}

func TestExtractZipFile(t *testing.T) {
	tempDir := t.TempDir()
	zipPath := filepath.Join(tempDir, "test.zip")
	extractDir := filepath.Join(tempDir, "extract")
	require.NoError(t, os.MkdirAll(extractDir, 0o755))

	// Create a ZIP with a simple file.
	createTestZip(t, zipPath, map[string][]byte{
		"hello.txt": []byte("hello world"),
		"build/app": []byte("binary content"),
	})

	err := extractZipFile(zipPath, extractDir)
	require.NoError(t, err)

	// Verify extracted files.
	content, err := os.ReadFile(filepath.Join(extractDir, "hello.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(content))

	content, err = os.ReadFile(filepath.Join(extractDir, "build", "app"))
	require.NoError(t, err)
	assert.Equal(t, "binary content", string(content))
}

func TestExtractZipFile_InvalidZip(t *testing.T) {
	tempDir := t.TempDir()
	notAZip := filepath.Join(tempDir, "not.zip")
	require.NoError(t, os.WriteFile(notAZip, []byte("this is not a zip"), 0o644))

	extractDir := filepath.Join(tempDir, "extract")
	require.NoError(t, os.MkdirAll(extractDir, 0o755))

	err := extractZipFile(notAZip, extractDir)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrPRArtifactExtractFailed)
}

func TestExtractZipFile_EmptyZip(t *testing.T) {
	tempDir := t.TempDir()
	zipPath := filepath.Join(tempDir, "empty.zip")
	extractDir := filepath.Join(tempDir, "extract")
	require.NoError(t, os.MkdirAll(extractDir, 0o755))

	// Create empty ZIP.
	createTestZip(t, zipPath, map[string][]byte{})

	err := extractZipFile(zipPath, extractDir)
	assert.NoError(t, err)
}

func TestInstallArtifactBinaryToDir(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := setupTestInstallPath(t, tempDir)
	defer cleanup()

	// Determine the binary name for this platform.
	binaryName := "atmos"
	if runtime.GOOS == "windows" {
		binaryName = "atmos.exe"
	}

	// Create a ZIP with the binary in the "build/" subdirectory.
	zipDir := filepath.Join(tempDir, "zips")
	require.NoError(t, os.MkdirAll(zipDir, 0o755))
	zipPath := filepath.Join(zipDir, "artifact.zip")

	createTestZip(t, zipPath, map[string][]byte{
		"build/" + binaryName: []byte("#!/bin/sh\necho atmos"),
	})

	binaryPath, err := installArtifactBinaryToDir("test-version", zipPath)
	require.NoError(t, err)
	assert.Contains(t, binaryPath, "test-version")
	assert.Contains(t, binaryPath, binaryName)

	// Verify binary exists and has content.
	content, err := os.ReadFile(binaryPath)
	require.NoError(t, err)
	assert.Equal(t, "#!/bin/sh\necho atmos", string(content))

	// Verify executable permission on Unix.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(binaryPath)
		require.NoError(t, err)
		assert.True(t, info.Mode()&0o100 != 0, "binary should be executable")
	}
}

func TestInstallArtifactBinaryToDir_RootLevel(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := setupTestInstallPath(t, tempDir)
	defer cleanup()

	binaryName := "atmos"
	if runtime.GOOS == "windows" {
		binaryName = "atmos.exe"
	}

	// Create a ZIP with the binary at the root level (no build/ subdirectory).
	zipDir := filepath.Join(tempDir, "zips")
	require.NoError(t, os.MkdirAll(zipDir, 0o755))
	zipPath := filepath.Join(zipDir, "artifact.zip")

	createTestZip(t, zipPath, map[string][]byte{
		binaryName: []byte("binary at root"),
	})

	binaryPath, err := installArtifactBinaryToDir("test-root", zipPath)
	require.NoError(t, err)
	assert.NotEmpty(t, binaryPath)

	content, err := os.ReadFile(binaryPath)
	require.NoError(t, err)
	assert.Equal(t, "binary at root", string(content))
}

func TestInstallArtifactBinaryToDir_NoBinary(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := setupTestInstallPath(t, tempDir)
	defer cleanup()

	// Create a ZIP without the expected binary.
	zipDir := filepath.Join(tempDir, "zips")
	require.NoError(t, os.MkdirAll(zipDir, 0o755))
	zipPath := filepath.Join(zipDir, "artifact.zip")

	createTestZip(t, zipPath, map[string][]byte{
		"README.md":  []byte("# readme"),
		"config.yml": []byte("key: value"),
	})

	_, err := installArtifactBinaryToDir("test-nobinary", zipPath)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrPRArtifactExtractFailed)
}

func TestCopyFile(t *testing.T) {
	tempDir := t.TempDir()
	srcPath := filepath.Join(tempDir, "source.txt")
	dstPath := filepath.Join(tempDir, "dest.txt")

	content := []byte("file content to copy")
	require.NoError(t, os.WriteFile(srcPath, content, 0o644))

	err := copyFile(srcPath, dstPath)
	require.NoError(t, err)

	copied, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	assert.Equal(t, content, copied)
}

func TestCopyFile_SourceNotFound(t *testing.T) {
	tempDir := t.TempDir()
	err := copyFile(filepath.Join(tempDir, "nonexistent"), filepath.Join(tempDir, "dest"))
	assert.Error(t, err)
}

func TestListFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create nested structure.
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "a", "b"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "file1.txt"), []byte("1"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "a", "file2.txt"), []byte("2"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "a", "b", "file3.txt"), []byte("3"), 0o644))

	files, err := listFiles(tempDir)
	require.NoError(t, err)
	assert.Contains(t, files, ".")
	assert.Contains(t, files, "file1.txt")
	assert.Contains(t, files, filepath.Join("a", "file2.txt"))
	assert.Contains(t, files, filepath.Join("a", "b", "file3.txt"))
}

func TestListFiles_EmptyDir(t *testing.T) {
	tempDir := t.TempDir()

	files, err := listFiles(tempDir)
	require.NoError(t, err)
	// Should contain at least the root dir entry.
	assert.Contains(t, files, ".")
	assert.Len(t, files, 1)
}

func TestSavePRCacheMetadataAfterInstall(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := setupTestInstallPath(t, tempDir)
	defer cleanup()

	// Create the PR directory structure.
	prDir := filepath.Join(tempDir, "bin", "cloudposse", "atmos", "pr-2040")
	require.NoError(t, os.MkdirAll(prDir, 0o755))

	info := &github.PRArtifactInfo{
		PRNumber:     2040,
		HeadSHA:      "abc123def456789",
		RunID:        99999,
		ArtifactID:   12345,
		ArtifactName: "build-artifacts-macos",
		SizeInBytes:  1024,
	}

	SavePRCacheMetadataAfterInstall(2040, info)

	// Verify file was created.
	cacheFile := filepath.Join(prDir, cacheMetadataFile)
	assert.FileExists(t, cacheFile)

	// Read back and verify.
	data, err := os.ReadFile(cacheFile)
	require.NoError(t, err)

	var loaded PRCacheMetadata
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.Equal(t, "abc123def456789", loaded.HeadSHA)
	assert.Equal(t, int64(99999), loaded.RunID)
}

func TestCheckPRCacheAndUpdate_NoMetadata(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := setupTestInstallPath(t, tempDir)
	defer cleanup()

	// No metadata exists for this PR.
	ctx := context.Background()
	needsReinstall, err := CheckPRCacheAndUpdate(ctx, 12345, false)

	// When no metadata exists, it should return true (needs reinstall) with nil error.
	assert.True(t, needsReinstall)
	assert.NoError(t, err)
}

func TestExtractZipFile_WithDirectory(t *testing.T) {
	tempDir := t.TempDir()
	zipPath := filepath.Join(tempDir, "test.zip")
	extractDir := filepath.Join(tempDir, "extract")
	require.NoError(t, os.MkdirAll(extractDir, 0o755))

	// Create a ZIP with a directory entry and a nested file.
	f, err := os.Create(zipPath)
	require.NoError(t, err)

	w := zip.NewWriter(f)
	// Add a directory entry.
	_, err = w.Create("subdir/")
	require.NoError(t, err)
	// Add a file in the directory.
	fw, err := w.Create("subdir/file.txt")
	require.NoError(t, err)
	_, err = fw.Write([]byte("nested content"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	err = extractZipFile(zipPath, extractDir)
	require.NoError(t, err)

	// Verify the directory was created.
	info, err := os.Stat(filepath.Join(extractDir, "subdir"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Verify the nested file was extracted.
	content, err := os.ReadFile(filepath.Join(extractDir, "subdir", "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "nested content", string(content))
}

func TestCopyFile_DestDirNotFound(t *testing.T) {
	tempDir := t.TempDir()
	srcPath := filepath.Join(tempDir, "source.txt")
	require.NoError(t, os.WriteFile(srcPath, []byte("content"), 0o644))

	// Destination directory doesn't exist.
	dstPath := filepath.Join(tempDir, "nonexistent", "dest.txt")
	err := copyFile(srcPath, dstPath)
	assert.Error(t, err)
}

func TestInstallArtifactBinaryToDir_InvalidZip(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := setupTestInstallPath(t, tempDir)
	defer cleanup()

	// Create an invalid zip file.
	zipDir := filepath.Join(tempDir, "zips")
	require.NoError(t, os.MkdirAll(zipDir, 0o755))
	zipPath := filepath.Join(zipDir, "bad.zip")
	require.NoError(t, os.WriteFile(zipPath, []byte("not a zip"), 0o644))

	_, err := installArtifactBinaryToDir("test-bad", zipPath)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrPRArtifactExtractFailed)
}

func TestSanitizeZipPath_AbsoluteWindowsPath(t *testing.T) {
	baseDir := t.TempDir()
	destDir := filepath.Clean(baseDir) + string(os.PathSeparator)

	// Backslash path should be rejected.
	_, err := sanitizeZipPath("dir\\file.txt", destDir)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrPRArtifactExtractFailed)
}

func TestHandlePRArtifactError_ContainsPRURL(t *testing.T) {
	// Verify the error wraps the expected sentinel error.
	err := handlePRArtifactError(assert.AnError, 2038)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrToolInstall)
}

func TestBuildTokenRequiredError_ContainsHints(t *testing.T) {
	err := buildTokenRequiredError()
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
}

// Note: Full integration tests for InstallFromPR require:
// - A valid GitHub token
// - Network access
// - A real PR with artifacts
// Those tests should be in a separate integration test file with appropriate
// skip conditions.
