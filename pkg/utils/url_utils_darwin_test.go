//go:build darwin
// +build darwin

package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenUrl_Darwin_ErrorWhenOpenMissing(t *testing.T) {
	oldGoTest := os.Getenv("GO_TEST")
	_ = os.Setenv("GO_TEST", "0")
	defer os.Setenv("GO_TEST", oldGoTest)

	oldPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", "")
	defer os.Setenv("PATH", oldPath)

	if err := OpenUrl("https://example.com"); err == nil {
		t.Fatalf("expected error when 'open' is not found on PATH")
	}
}

func TestOpenUrl_Darwin_UsesOpen_Success(t *testing.T) {
	tmp := t.TempDir()
	outFile := filepath.Join(tmp, "open-called.txt")
	fake := filepath.Join(tmp, "open")

	script := fmt.Sprintf("#\!/bin/sh\necho \"$1\" > %q\n", outFile)
	if err := os.WriteFile(fake, []byte(script), 0o700); err \!= nil {
		t.Fatalf("failed to write fake open: %v", err)
	}

	oldGoTest := os.Getenv("GO_TEST")
	_ = os.Setenv("GO_TEST", "0")
	defer os.Setenv("GO_TEST", oldGoTest)

	oldPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", tmp)
	defer os.Setenv("PATH", oldPath)

	url := "https://example.com/mac"
	if err := OpenUrl(url); err \!= nil {
		t.Fatalf("expected nil error with fake open, got %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for fake open to record invocation")
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