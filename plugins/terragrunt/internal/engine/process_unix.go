//go:build !windows

package engine

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// processTree gives each invocation its own group so cancellation reaches tool children too.
type processTree struct{ cmd *exec.Cmd }

func prepareProcessTree(cmd *exec.Cmd) (*processTree, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	return &processTree{cmd: cmd}, nil
}

func (*processTree) started() error { return nil }

// A version probe never owns persistent services, including children holding pipes open.
func (t *processTree) close() {
	if t.cmd.Process != nil {
		_ = syscall.Kill(-t.cmd.Process.Pid, syscall.SIGKILL)
	}
}
