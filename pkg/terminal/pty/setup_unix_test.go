//go:build !windows

package pty

import (
	"os"
	"testing"

	"github.com/creack/pty"
)

// TestSetupTerminalCleanupStopsResizeGoroutine verifies the SIGWINCH goroutine started by
// setupTerminal has stopped, and no longer reads os.Stdin, by the time cleanup returns.
// Tests reassign the os.Stdin global right after a PTY session ends, so a goroutine that
// outlives cleanup races with that write (detected by `go test -race`).
func TestSetupTerminalCleanupStopsResizeGoroutine(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("PTY not available: %v", err)
	}
	t.Cleanup(func() {
		_ = ptmx.Close()
		_ = tty.Close()
	})

	replacement, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("os.Open(%q) error = %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = replacement.Close() })

	original := os.Stdin
	t.Cleanup(func() { os.Stdin = original })

	for range 100 {
		cleanup, err := setupTerminal(ptmx, false)
		if err != nil {
			t.Fatalf("setupTerminal() error = %v", err)
		}
		cleanup()

		// Reassigning the global after cleanup must not race with the resize goroutine.
		os.Stdin = replacement
		os.Stdin = original
	}
}
