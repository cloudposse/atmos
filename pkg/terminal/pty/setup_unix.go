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

	"github.com/cloudposse/atmos/pkg/signals"
)

// setupTerminal configures terminal resize handling and raw mode.
// The rawMode parameter controls whether the host terminal is switched to raw
// mode; it is only needed when host input is forwarded to the PTY.
// Returns a cleanup function that must be called when done.
func setupTerminal(ptmx *os.File, rawMode bool) (func(), error) {
	// Handle terminal resize signals. The goroutine uses the stdin captured here rather than
	// reading the os.Stdin global, which callers (notably tests) may reassign, and signals on
	// done when it exits so stopResize can wait for it.
	stdin := os.Stdin
	ch := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		defer close(done)
		for range ch {
			_ = pty.InheritSize(stdin, ptmx)
		}
	}()
	// stopResize stops signal delivery and waits for the resize goroutine to exit, so it can
	// no longer touch stdin or ptmx after the caller releases them.
	stopResize := func() {
		signal.Stop(ch)
		close(ch)
		<-done
	}
	ch <- syscall.SIGWINCH // Initial resize.

	// Set terminal to raw mode (only if requested and stdin is a TTY).
	var oldState *term.State
	var err error
	if rawMode && term.IsTerminal(int(os.Stdin.Fd())) { //nolint:gosec // Stdin fd fits in int on all supported platforms.
		oldState, err = term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			stopResize()
			return nil, fmt.Errorf("failed to set terminal to raw mode: %w", err)
		}
	}

	// Restore the terminal even if the process exits through the signal
	// handler (os.Exit skips deferred functions).
	deregister := func() {}
	if oldState != nil {
		state := oldState
		deregister = signals.RegisterExitCleanup(func() {
			_ = term.Restore(int(os.Stdin.Fd()), state) //nolint:gosec // Stdin fd fits in int on all supported platforms.
		})
	}

	// Return cleanup function.
	cleanup := func() {
		deregister()
		stopResize()
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
