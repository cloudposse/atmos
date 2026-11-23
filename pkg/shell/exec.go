// Package shell provides generic helpers for launching subprocesses and
// interactive shells with a caller-supplied environment. It is intentionally
// free of any auth/secret-specific knowledge so multiple commands
// (e.g. `atmos auth exec`, `atmos secret exec`) can reuse it.
package shell

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RunCommand executes args[0] with args[1:] using the supplied environment
// list, wiring stdin/stdout/stderr to the parent process.
//
// The env list is passed to the child verbatim and must already be complete
// (e.g. os.Environ() + any additions) — do NOT prepend os.Environ() or convert
// to a map, as that would lose ordering and could collide on duplicate keys
// (such as Windows-style drive-scoped vars).
//
// The child's exit code is propagated as errUtils.ExitCodeError so the root
// command can exit with the same status. A missing executable returns
// errUtils.ErrCommandNotFound with exit code 127 (the conventional shell
// "command not found" code), distinct from ErrUnknownSubcommand so the root
// handler does not mistake it for an unknown Atmos subcommand.
func RunCommand(args []string, env []string) error {
	defer perf.Track(nil, "shell.RunCommand")()

	if len(args) == 0 {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrNoCommandSpecified, errUtils.ErrInvalidSubcommand)
	}

	// Prepare the command.
	cmdName := args[0]
	cmdArgs := args[1:]

	// Look for the command in PATH.
	cmdPath, err := exec.LookPath(cmdName)
	if err != nil {
		return errUtils.Build(errUtils.ErrCommandNotFound).
			WithCause(err).
			WithContext("command", cmdName).
			WithHintf("Ensure %q is installed and available on your PATH", cmdName).
			WithExitCode(127).
			Err()
	}

	// Execute the command.
	// #nosec G204 -- This is intentional: exec is designed to run user-specified commands.
	execCmd := exec.Command(cmdPath, cmdArgs...)
	execCmd.Env = env
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	// Start the command, keep it in step with signals sent to Atmos (see
	// superviseChild), and wait for completion.
	if err = execCmd.Start(); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrSubcommandFailed, err)
	}
	stopSupervision := superviseChild(execCmd.Process, commandTerminationSignals)
	err = execCmd.Wait()
	stopSupervision()
	if err != nil {
		// If it's an exit error, propagate as a typed error so the root can exit with the same code.
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				return errUtils.ExitCodeError{Code: childExitCode(status)}
			}
		}
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrSubcommandFailed, err)
	}

	return nil
}

// signalExitCodeBase is added to a signal number to form the exit status of a
// child killed by that signal, following the POSIX shell convention (128+N).
const signalExitCodeBase = 128

// childExitCode returns the exit status to propagate for a finished child.
// ExitStatus() is -1 for a child terminated by a signal, which the process
// would report as 255, so a signaled child maps to 128+N (for example 137 for
// SIGKILL) like a shell does. Windows has no POSIX signals, so Signaled() is
// always false there and the child's own exit code is used.
func childExitCode(status syscall.WaitStatus) int {
	if status.Signaled() {
		return signalExitCodeBase + int(status.Signal())
	}
	return status.ExitStatus()
}
