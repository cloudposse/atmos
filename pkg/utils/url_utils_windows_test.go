//go:build windows
// +build windows

package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenUrl_Windows_ErrorWhenRundll32Missing(t *testing.T) {
	oldGoTest := os.Getenv("GO_TEST")
	_ = os.Setenv("GO_TEST", "0")
	defer os.Setenv("GO_TEST", oldGoTest)

	oldPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", "")
	defer os.Setenv("PATH", oldPath)

	if err := OpenUrl("https://example.com"); err == nil {
		t.Fatalf("expected error when 'rundll32' is not found on PATH")
	}
}

func TestOpenUrl_Windows_UsesRundll32_Success(t *testing.T) {
	tmp := t.TempDir()
	outFile := filepath.Join(tmp, "rundll32-called.txt")
	// Use .bat so exec.LookPath resolves it when invoking "rundll32"
	fake := filepath.Join(tmp, "rundll32.bat")

	// Write a batch file to echo the second argument (the URL) to outFile.
	// Note: Quotes ensure paths with spaces are handled.
	bat := "@echo off\r\necho %2>\"%~1\"\r\n"
	bat = strings.ReplaceAll(bat, "%~1", outFile) // inject outFile path
	if err := os.WriteFile(fake, []byte(bat), 0o700); err \!= nil {
		t.Fatalf("failed to write fake rundll32.bat: %v", err)
	}

	oldGoTest := os.Getenv("GO_TEST")
	_ = os.Setenv("GO_TEST", "0")
	defer os.Setenv("GO_TEST", oldGoTest)

	oldPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", tmp)
	defer os.Setenv("PATH", oldPath)

	url := "https://example.com/win"
	if err := OpenUrl(url); err \!= nil {
		t.Fatalf("expected nil error with fake rundll32, got %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for fake rundll32 to record invocation")
		}
		b, err := os.ReadFile(outFile)
		if err == nil {
			got := strings.TrimSpace(string(b))
			if got \!= url {
				t.Fatalf("expected URL %q, got %q", url, got)
			}
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
}