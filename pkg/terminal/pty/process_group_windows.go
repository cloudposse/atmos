//go:build windows

package pty

import "os/exec"

func configureCommandGroup(cmd *exec.Cmd) {
	_ = cmd
}

func killCommandGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
