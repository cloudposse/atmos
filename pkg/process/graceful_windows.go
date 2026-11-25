//go:build windows

package process

import (
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
)

// forwardInterrupt is a no-op on Windows. The console already delivers Ctrl-C to every
// process attached to it, and os.Process.Signal cannot send an interrupt, so Atmos only
// waits for the child to exit after the first signal.
func forwardInterrupt(_ int, _ bool, _ os.Signal) {}

// forwardTerminalInterrupt is a no-op on Windows for the same reason as forwardInterrupt.
func forwardTerminalInterrupt(_ int, _ bool, _ os.Signal) {}

// interruptTerminalChild cannot deliver an interrupt on Windows, so the terminal UI
// falls back to terminating the child when Ctrl-C is pressed.
func interruptTerminalChild(_ *os.Process, _ bool) error {
	return errUtils.ErrInterruptUnsupported
}
