package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestOpenUrl_SkipsInTestEnv verifies that when GO_TEST=1 is set,
// OpenUrl immediately returns nil without attempting to launch a browser.
// We can't directly assert that exec.Command wasn't called, but the early return
// is deterministic and OS-agnostic.
func TestOpenUrl_SkipsInTestEnv(t *testing.T) {
	t.Parallel()
	t.Setenv("GO_TEST", "1")

	// Ensure any previous viper state does not interfere; OpenUrl calls BindEnv each time.
	// Call with typical URL
	if err := OpenUrl("https://example.com"); err \!= nil {
		t.Fatalf("expected nil error when GO_TEST=1, got %v", err)
	}

	// Call with empty URL to ensure no hidden validation occurs under GO_TEST=1
	if err := OpenUrl(""); err \!= nil {
		t.Fatalf("expected nil error for empty url when GO_TEST=1, got %v", err)
	}

	// Call with odd characters
	if err := OpenUrl("not a url \!\!\!"); err \!= nil {
		t.Fatalf("expected nil error for arbitrary string when GO_TEST=1, got %v", err)
	}
}

// TestOpenUrl_Linux_UsesXdgOpen_Succeeds stubs an xdg-open binary on PATH so that
// Start() succeeds and OpenUrl returns nil. Linux-only.
func TestOpenUrl_Linux_UsesXdgOpen_Succeeds(t *testing.T) {
	if runtime.GOOS \!= "linux" {
		t.Skip("linux-specific test")
	}
	t.Parallel()

	// Ensure GO_TEST is not set to bypass the early return.
	t.Setenv("GO_TEST", "0")

	tmpDir := t.TempDir()
	xdg := filepath.Join(tmpDir, "xdg-open")

	// Create a stub xdg-open that exits immediately with success.
	// Use a POSIX sh script; Start() should succeed and return nil from OpenUrl.
	if err := os.WriteFile(xdg, []byte("#\!/bin/sh\nexit 0\n"), 0o755); err \!= nil {
		t.Fatalf("failed to create stub xdg-open: %v", err)
	}

	// Prepend our stub directory to PATH
	path := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+path)

	if err := OpenUrl("https://example.com"); err \!= nil {
		t.Fatalf("expected nil error when stub xdg-open is present, got %v", err)
	}
}

// TestOpenUrl_Linux_UsesXdgOpen_StartFails makes xdg-open non-executable so that Start() fails,
// and OpenUrl should return an error. Linux-only.
func TestOpenUrl_Linux_UsesXdgOpen_StartFails(t *testing.T) {
	if runtime.GOOS \!= "linux" {
		t.Skip("linux-specific test")
	}
	t.Parallel()

	t.Setenv("GO_TEST", "0")

	tmpDir := t.TempDir()
	xdg := filepath.Join(tmpDir, "xdg-open")

	// Create a non-executable file; Start should fail with a permission error.
	if err := os.WriteFile(xdg, []byte("not-executable"), 0o644); err \!= nil {
		t.Fatalf("failed to create non-executable xdg-open: %v", err)
	}

	path := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+path)

	if err := OpenUrl("https://example.com"); err == nil {
		t.Fatalf("expected error when xdg-open is non-executable, got nil")
	}
}

// TestOpenUrl_Darwin_UsesOpen_Succeeds stubs an 'open' binary on PATH to simulate success. Darwin-only.
func TestOpenUrl_Darwin_UsesOpen_Succeeds(t *testing.T) {
	if runtime.GOOS \!= "darwin" {
		t.Skip("darwin-specific test")
	}
	t.Parallel()

	t.Setenv("GO_TEST", "0")

	tmpDir := t.TempDir()
	openBin := filepath.Join(tmpDir, "open")

	if err := os.WriteFile(openBin, []byte("#\!/bin/sh\nexit 0\n"), 0o755); err \!= nil {
		t.Fatalf("failed to create stub open: %v", err)
	}

	path := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+path)

	if err := OpenUrl("https://example.com"); err \!= nil {
		t.Fatalf("expected nil error when stub open is present, got %v", err)
	}
}

// TestOpenUrl_Darwin_UsesOpen_StartFails makes 'open' non-executable so Start() fails. Darwin-only.
func TestOpenUrl_Darwin_UsesOpen_StartFails(t *testing.T) {
	if runtime.GOOS \!= "darwin" {
		t.Skip("darwin-specific test")
	}
	t.Parallel()

	t.Setenv("GO_TEST", "0")

	tmpDir := t.TempDir()
	openBin := filepath.Join(tmpDir, "open")

	if err := os.WriteFile(openBin, []byte("not-executable"), 0o644); err \!= nil {
		t.Fatalf("failed to create non-executable open: %v", err)
	}

	path := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+path)

	if err := OpenUrl("https://example.com"); err == nil {
		t.Fatalf("expected error when open is non-executable, got nil")
	}
}

// Note: Windows branch involves invoking "rundll32". Creating a reliable cross-platform stub
// for Windows within *nix CI is non-trivial due to PATHEXT and .exe requirements.
// If Windows CI is enabled, analogous tests can be added using a temporary rundll32.exe stub
// or a .cmd/.bat shim, and guarded by runtime.GOOS == "windows".