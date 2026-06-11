//go:build windows

package process

import (
	"fmt"
	"os"
	"os/exec"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/signals"
)

// ReplaceShellSession emulates process replacement on Windows, which has no
// execve: the command is spawned with the real standard streams inherited,
// Atmos waits for it (ignoring Ctrl-C, which the child owns), and the child's
// exit code is returned as errUtils.ExitCodeError. Because exec steps are
// validated to be the final step, propagating the exit code is observably
// equivalent to true replacement.
func ReplaceShellSession(spec *ExecSpec) error {
	defer perf.Track(nil, "process.ReplaceShellSession")()

	env, proceed, err := prepareExec(spec)
	if err != nil || !proceed {
		return err
	}

	// The child owns Ctrl-C for the remainder of the process lifetime.
	release := signals.SuspendInterruptExit()
	defer release()

	shell, flag := sessionShell()
	cmd := exec.Command(shell, flag, spec.Command)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrProcessStartFailed, err)
	}
	return sessionExitError(cmd.Wait())
}
