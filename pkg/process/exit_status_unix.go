//go:build !windows

package process

import (
	"os/exec"
	"syscall"
)

// signalExitBase is the POSIX shell convention base for signal-death exit
// codes: a child killed by signal N reports 128+N (e.g. 130 for SIGINT).
const signalExitBase = 128

// exitStatusCode maps a child exit to a shell-convention exit code: children
// killed by a signal report 128+signal (e.g. 130 for SIGINT) instead of Go's
// -1, matching what users see from their shell.
func exitStatusCode(exitErr *exec.ExitError) int {
	if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return signalExitBase + int(ws.Signal())
	}
	return exitErr.ExitCode()
}
