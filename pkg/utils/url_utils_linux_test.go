//go:build linux
// +build linux

package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestOpenUrl_Linux_ErrorWhenXdgOpenMissing ensures an error is returned if xdg-open is not found on PATH.
func TestOpenUrl_Linux_ErrorWhenXdgOpenMissing(t *testing.T) {
	oldGoTest := os.Getenv("GO_TEST")
	_ = os.Setenv("GO_TEST", "0")
	defer os.Setenv("GO_TEST", oldGoTest)

	oldPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", "")
	defer os.Setenv("PATH", oldPath)

	if err := OpenUrl("https://example.com"); err == nil {
		t.Fatalf("expected error when xdg-open is not found on PATH")
	}
}

// TestOpenUrl_Linux_UsesXdgOpen_Success fakes xdg-open to verify that the correct command is launched with the URL.
func TestOpenUrl_Linux_UsesXdgOpen_Success(t *testing.T) {
	tmp := t.TempDir()
	outFile := filepath.Join(tmp, "xdg-open-called.txt")
	fake := filepath.Join(tmp, "xdg-open")

	script := fmt.Sprintf("#\!/bin/sh\necho \"$1\" > %q\n", outFile)
	if err := os.WriteFile(fake, []byte(script), 0o700); err \!= nil {
		t.Fatalf("failed to write fake xdg-open: %v", err)
	}

	oldGoTest := os.Getenv("GO_TEST")
	_ = os.Setenv("GO_TEST", "0")
	defer os.Setenv("GO_TEST", oldGoTest)

	oldPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", tmp)
	defer os.Setenv("PATH", oldPath)

	url := "https://example.com/foo?bar=baz"
	if err := OpenUrl(url); err \!= nil {
		t.Fatalf("expected nil error with fake xdg-open, got %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for fake xdg-open to record invocation")
		}
		b, err := os.ReadFile(outFile)
		if err == nil {
			got := strings.TrimSpace(string(b))
			if got \!= url {
				t.Fatalf("expected URL %q, got %q", url, got)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestOpenUrl_Linux_EmptyURL_StillAttemptsLaunch verifies behavior with an empty URL argument; an error is still expected when launcher missing.
func TestOpenUrl_Linux_EmptyURL_StillAttemptsLaunch(t *testing.T) {
	oldGoTest := os.Getenv("GO_TEST")
	_ = os.Setenv("GO_TEST", "0")
	defer os.Setenv("GO_TEST", oldGoTest)

	oldPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", "")
	defer os.Setenv("PATH", oldPath)

	if err := OpenUrl(""); err == nil {
		t.Fatalf("expected error when xdg-open is not found on PATH even with empty URL")
	}
}