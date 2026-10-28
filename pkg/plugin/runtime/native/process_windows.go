//go:build windows

package native

import "os/exec"

// configureProcess keeps the default behavior on Windows: cancellation terminates the
// plugin process. Windows has no process-group signal, so descendants are not tracked.
func configureProcess(command *exec.Cmd) {
	command.WaitDelay = terminationGrace
}

// killProcessGroup has nothing to add on Windows; the process itself is killed by go-plugin.
func killProcessGroup(*exec.Cmd) {}
