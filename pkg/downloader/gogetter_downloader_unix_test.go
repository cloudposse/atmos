//go:build !windows
// +build !windows

package downloader

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestGoGetterGet_File verifies file:// URL handling on Unix-like systems.
func TestGoGetterGet_File(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "src")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(srcDir)
	srcFile := filepath.Join(srcDir, "test.txt")
	content := []byte("hello world")
	if err := os.WriteFile(srcFile, content, 0o600); err != nil {
		t.Fatal(err)
	}
	destDir, err := os.MkdirTemp("", "dest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(destDir)
	destFile := filepath.Join(destDir, "downloaded.txt")
	srcURL := "file://" + srcFile
	config := fakeAtmosConfig()
	err = NewGoGetterDownloader(&config).Fetch(srcURL, destFile, ClientModeFile, 5*time.Second)
	if err != nil {
		t.Errorf("GoGetterGet failed: %v", err)
	}
	data, err := os.ReadFile(destFile)
	if err != nil {
		t.Errorf("Error reading downloaded file: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("Expected file content %s, got %s", content, data)
	}
}
