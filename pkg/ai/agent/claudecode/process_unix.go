//go:build !windows

package claudecode

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// processTree gives each invocation its own group so cancellation reaches tool children too.
type processTree struct{}

func prepareProcessTree(cmd *exec.Cmd) (*processTree, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	return &processTree{}, nil
}

func (*processTree) started() error { return nil }

// Normal completion does not kill services deliberately detached by a successful invocation.
func (*processTree) close() {}
