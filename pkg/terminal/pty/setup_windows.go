//go:build windows

package pty

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// setupTerminal configures terminal for Windows.
// Returns a cleanup function that must be called when done.
// Note: Windows doesn't support SIGWINCH for resize handling.
func setupTerminal(ptmx *os.File) (func(), error) {
	_ = ptmx // ptmx is used in Unix implementation but not needed on Windows

	// Set terminal to raw mode (only if stdin is a TTY).
	var oldState *term.State
	var err error
	if term.IsTerminal(int(os.Stdin.Fd())) {
		oldState, err = term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return nil, fmt.Errorf("failed to set terminal to raw mode: %w", err)
		}
	}

	// Return cleanup function.
	cleanup := func() {
		if oldState != nil {
			_ = term.Restore(int(os.Stdin.Fd()), oldState)
		}
	}

	return cleanup, nil
}

// isPtyEIO checks if an error is from a closed PTY.
// On Windows, we don't have syscall.EIO, so we return false.
func isPtyEIO(err error) bool {
	// Windows doesn't have EIO, but PTY errors manifest differently.
	// For now, return false as PTY support on Windows is limited.
	return false
}
