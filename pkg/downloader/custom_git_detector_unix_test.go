//go:build !windows
// +build !windows

package downloader

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRemoveSymlinks verifies that symlinks are properly removed on Unix-like systems.
func TestRemoveSymlinks(t *testing.T) {
	tempDir := t.TempDir()

	filePath := filepath.Join(tempDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(tempDir, "link.txt")
	if err := os.Symlink(filePath, symlinkPath); err != nil {
		t.Fatal(err)
	}
	if err := removeSymlinks(tempDir); err != nil {
		t.Fatalf("removeSymlinks error: %v", err)
	}
	if _, err := os.Lstat(symlinkPath); !os.IsNotExist(err) {
		t.Errorf("Expected symlink to be removed, but it exists")
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Errorf("Expected regular file to exist, but got error: %v", err)
	}
}
