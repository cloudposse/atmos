//go:build !windows
// +build !windows

package exec

import (
	"os"
	"testing"
)

// TestCreateTempDirectory verifies Unix-specific directory permissions.
func TestCreateTempDirectory(t *testing.T) {
	dir, err := createTempDirectory()
	if err != nil {
		t.Fatalf("createTempDirectory returned error: %v", err)
	}
	defer os.RemoveAll(dir)

	// Check that the directory exists.
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("failed to stat directory: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("Expected a directory, got a file")
	}

	// Check that the permissions are exactly defaultDirPermissions.
	mode := info.Mode().Perm()
	if mode != defaultDirPermissions {
		t.Errorf("Expected mode %o, got %o", defaultDirPermissions, mode)
	}
}
