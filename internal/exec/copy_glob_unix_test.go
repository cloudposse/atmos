//go:build !windows
// +build !windows

package exec

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestShouldExcludePath_Directory checks directory exclusion using a trailing slash on Unix.
func TestShouldExcludePath_Directory(t *testing.T) {
	dir := t.TempDir()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Failed to stat directory: %v", err)
	}
	pattern := "**/" + filepath.Base(dir) + "/"
	if !shouldExcludePath(info, filepath.Base(dir), []string{pattern}) {
		t.Errorf("Expected directory %q to be excluded by pattern %q", filepath.Base(dir), pattern)
	}
}

// TestProcessDirEntry_Symlink ensures that symlink entries are skipped on Unix.
func TestProcessDirEntry_Symlink(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	targetFile := filepath.Join(srcDir, "target.txt")
	if err := os.WriteFile(targetFile, []byte("data"), 0o600); err != nil {
		t.Fatalf("Failed to write target file: %v", err)
	}
	linkPath := filepath.Join(srcDir, "link.txt")
	if err := os.Symlink(targetFile, linkPath); err != nil {
		t.Skipf("Cannot create symlink on this system: insufficient privileges or unsupported filesystem")
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatalf("Failed to read src dir: %v", err)
	}
	var linkEntry os.DirEntry
	for _, e := range entries {
		if e.Name() == "link.txt" {
			linkEntry = e
			break
		}
	}
	if linkEntry == nil {
		t.Fatalf("Symlink entry not found")
	}
	ctx := &CopyContext{
		SrcDir:   srcDir,
		DstDir:   dstDir,
		BaseDir:  srcDir,
		Excluded: []string{},
		Included: []string{},
	}
	if err := processDirEntry(linkEntry, ctx); err != nil {
		t.Errorf("Expected nil error for symlink, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "link.txt")); err == nil {
		t.Errorf("Expected symlink not to be copied")
	}
}

// TestGetMatchesForPattern_ShallowNoMatch tests the shallow branch with no matches on Unix.
func TestGetMatchesForPattern_ShallowNoMatch(t *testing.T) {
	oldFn := getGlobMatchesForTest
	defer func() { getGlobMatchesForTest = oldFn }()
	getGlobMatchesForTest = func(pattern string) ([]string, error) {
		normalized := filepath.ToSlash(pattern)
		if strings.Contains(normalized, "/*") && !strings.Contains(normalized, "/**") {
			return []string{}, nil
		}
		return []string{}, nil
	}
	srcDir := t.TempDir()
	emptyDir := filepath.Join(srcDir, "dir")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatalf("Failed to create empty directory: %v", err)
	}
	_, err := getMatchesForPatternForTest(srcDir, "dir/*")
	if err != nil {
		t.Fatalf("Expected no error for shallow pattern with no matches, got %v", err)
	}
}

// TestCopyFile_FailCreate tests error when creating destination file fails on Unix.
func TestCopyFile_FailCreate(t *testing.T) {
	srcDir := t.TempDir()

	dstDir := t.TempDir()

	// Create source file.
	srcFile := filepath.Join(srcDir, "test.txt")
	if err := os.WriteFile(srcFile, []byte("content"), 0o600); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}

	// Create a read-only destination directory to force os.Create to fail.
	if err := os.Chmod(dstDir, 0o500); err != nil {
		t.Fatalf("Failed to change dst dir permissions: %v", err)
	}
	defer os.Chmod(dstDir, 0o700) // Restore for cleanup.

	dstFile := filepath.Join(dstDir, "test.txt")
	err := copyFile(srcFile, dstFile)
	if err == nil {
		t.Errorf("Expected error when creating destination file, got nil")
	}
	if err != nil && !errors.Is(err, errUtils.ErrOpenFile) {
		t.Errorf("Expected ErrOpenFile, got %v", err)
	}
}

// TestShouldSkipPrefixEntry_File tests file exclusion in prefix mode on Unix.
func TestShouldSkipPrefixEntry_File(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	info, err := tmpFile.Stat()
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	// File should be excluded by pattern.
	if !shouldSkipPrefixEntry(info, "logs/test.log", []string{"**/logs/*.log"}) {
		t.Errorf("Expected file to be excluded by pattern")
	}
}
