//go:build !windows

package process

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

const managedWaitDelay = 5 * time.Second

func prepareManagedCommand(cmd *exec.Cmd) (func(), error) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	if cmd.Cancel != nil {
		cmd.Cancel = func() error {
			if cmd.Process == nil {
				return os.ErrProcessDone
			}
			err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			if errors.Is(err, syscall.ESRCH) {
				return os.ErrProcessDone
			}
			return err
		}
		cmd.WaitDelay = managedWaitDelay
	}
	return nil, nil
}

func activateManagedCommand(cmd *exec.Cmd) error {
	_ = cmd
	return nil
}

func killManagedCommand(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func ConfigureProcessGroup(cmd *exec.Cmd) {
	defer perf.Track(nil, "process.ConfigureProcessGroup")()

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func KillProcessGroup(cmd *exec.Cmd) error {
	defer perf.Track(nil, "process.KillProcessGroup")()

	return killManagedCommand(cmd)
}
