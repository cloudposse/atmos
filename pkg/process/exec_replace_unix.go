//go:build !windows

package process

import (
	"fmt"
	"os/exec"
	"syscall"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ReplaceShellSession replaces the Atmos process with the spec's command
// (execve of the system shell). On success it never returns: the command
// inherits the terminal, file descriptors, environment, and working
// directory, and its exit code becomes the process exit code natively.
// It returns an error only if the replacement could not be performed
// (or nil for dry-run).
func ReplaceShellSession(spec *ExecSpec) error {
	defer perf.Track(nil, "process.ReplaceShellSession")()

	env, proceed, err := prepareExec(spec)
	if err != nil || !proceed {
		return err
	}

	shell, flag := sessionShell()
	shellPath, err := exec.LookPath(shell)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrProcessStartFailed, err)
	}

	// On success, syscall.Exec does not return.
	if err := syscall.Exec(shellPath, []string{shell, flag, spec.Command}, env); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrProcessStartFailed, err)
	}
	return nil
}
