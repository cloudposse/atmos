//go:build !windows

package native

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// configureProcess places the plugin in its own process group so Atmos can stop it and
// anything it spawned, and replaces the default immediate kill on cancellation with a
// graceful SIGTERM followed by a forced kill after the grace period.
func configureProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if err := signalGroup(command, syscall.SIGTERM); err != nil {
			return command.Process.Kill()
		}
		return nil
	}
	command.WaitDelay = terminationGrace
}

// killProcessGroup forcibly ends every process left in the plugin's group.
func killProcessGroup(command *exec.Cmd) {
	_ = signalGroup(command, syscall.SIGKILL)
}

func signalGroup(command *exec.Cmd, signal syscall.Signal) error {
	if command == nil || command.Process == nil {
		return os.ErrProcessDone
	}
	err := syscall.Kill(-command.Process.Pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
