//go:build windows

package process

import "os"

// forwardInterrupt is a no-op on Windows. The console already delivers Ctrl-C to every
// process attached to it, and os.Process.Signal cannot send an interrupt, so Atmos only
// waits for the child to exit after the first signal.
func forwardInterrupt(_ int, _ bool, _ os.Signal) {}
