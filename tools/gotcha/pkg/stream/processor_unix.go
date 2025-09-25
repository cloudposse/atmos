//go:build !windows
// +build !windows

package stream

import (
	"os"
	"os/exec"
	"syscall"
)

// setPlatformSpecificCmd sets platform-specific command attributes for Unix systems.
func setPlatformSpecificCmd(cmd *exec.Cmd) {
	// On Unix systems, create a new process group
	// This allows us to kill all child processes when interrupted
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// killProcessGroup kills the process group on Unix systems.
func killProcessGroup(pid int) {
	if pid > 0 {
		// On Unix systems, kill the entire process group
		// The negative PID means kill the process group
		_ = syscall.Kill(-pid, syscall.SIGTERM)
	}
}

// getInterruptSignals returns the signals to listen for interruption.
func getInterruptSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}
