package exec

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hashicorp/go-getter"
)

var originalDetectors = getter.Detectors

func TestValidateURI(t *testing.T) {
	if err := ValidateURI(""); err == nil {
		t.Error("Expected error for empty URI, got nil")
	}
	uri := strings.Repeat("a", 2050)
	if err := ValidateURI(uri); err == nil {
		t.Error("Expected error for too-long URI, got nil")
	}
	if err := ValidateURI("http://example.com/../secret"); err == nil {
		t.Error("Expected error for path traversal sequence, got nil")
	}
	if err := ValidateURI("http://example.com/space test"); err == nil {
		t.Error("Expected error for spaces in URI, got nil")
	}
	if err := ValidateURI("http://example.com/path"); err != nil {
		t.Errorf("Expected valid URI, got error: %v", err)
	}
	if err := ValidateURI("oci://repo/path"); err != nil {
		t.Errorf("Expected valid OCI URI, got error: %v", err)
	}
	if err := ValidateURI("oci://repo"); err == nil {
		t.Error("Expected error for invalid OCI URI format, got nil")
	}
}

func TestIsValidScheme(t *testing.T) {
	valid := []string{"http", "https", "git", "ssh", "git::https", "git::ssh"}
	for _, scheme := range valid {
		if !IsValidScheme(scheme) {
			t.Errorf("Expected scheme %s to be valid", scheme)
		}
	}
	if IsValidScheme("ftp") {
		t.Error("Expected scheme ftp to be invalid")
	}
}

func TestRemoveSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping symlink tests on Windows.")
	}
	tempDir, err := os.MkdirTemp("", "symlinktest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
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

func TestValidateURI_ErrorPaths(t *testing.T) {
	err := ValidateURI("http://example.com/with space")
	if err == nil {
		t.Error("Expected error for URI with space")
	}
	err = ValidateURI("http://example.com/../secret")
	if err == nil {
		t.Error("Expected error for URI with path traversal")
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	getter.Detectors = originalDetectors
	os.Exit(code)
}
