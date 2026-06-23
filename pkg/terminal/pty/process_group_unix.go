//go:build !windows

package pty

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureCommandGroup(cmd *exec.Cmd) {
	_ = cmd
}

func killCommandGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return os.ErrProcessDone
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
