//go:build !windows

package pty

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// setupTerminal configures terminal resize handling and raw mode.
// Returns a cleanup function that must be called when done.
func setupTerminal(ptmx *os.File) (func(), error) {
	// Handle terminal resize signals.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			_ = pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	ch <- syscall.SIGWINCH // Initial resize.

	// Set terminal to raw mode (only if stdin is a TTY).
	var oldState *term.State
	var err error
	if term.IsTerminal(int(os.Stdin.Fd())) {
		oldState, err = term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			signal.Stop(ch)
			close(ch)
			return nil, fmt.Errorf("failed to set terminal to raw mode: %w", err)
		}
	}

	// Return cleanup function.
	cleanup := func() {
		signal.Stop(ch)
		close(ch)
		if oldState != nil {
			_ = term.Restore(int(os.Stdin.Fd()), oldState)
		}
	}

	return cleanup, nil
}

// isPtyEIO checks if an error is the expected EIO error from reading a closed PTY.
//
// The Linux kernel returns EIO when attempting to read from a master pseudo-terminal
// which no longer has an open slave. This is normal behavior and not an error condition.
//
// See: https://github.com/creack/pty/issues/21
func isPtyEIO(err error) bool {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return errors.Is(pathErr.Err, syscall.EIO)
	}
	return false
}
