//go:build windows
// +build windows

package stream

import (
	"os"
	"os/exec"
)

// setPlatformSpecificCmd sets platform-specific command attributes for Windows.
func setPlatformSpecificCmd(cmd *exec.Cmd) {
	// On Windows, we don't need to set process group attributes
	// Windows handles process groups differently
}

// killProcessGroup attempts to kill child processes on Windows.
func killProcessGroup(pid int) {
	// On Windows, we can't use negative PIDs to kill process groups
	// The cmd.Process.Kill() in the main code will handle process termination
	// Child processes will be terminated when the parent dies (usually)
}

// getInterruptSignals returns the signals to listen for interruption.
func getInterruptSignals() []os.Signal {
	// On Windows, we only have os.Interrupt (Ctrl+C)
	// SIGTERM doesn't exist on Windows
	return []os.Signal{os.Interrupt}
}
