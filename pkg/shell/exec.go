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

	// Run the command and wait for completion.
	err = execCmd.Run()
	if err != nil {
		// If it's an exit error, propagate as a typed error so the root can exit with the same code.
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				return errUtils.ExitCodeError{Code: status.ExitStatus()}
			}
		}
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrSubcommandFailed, err)
	}

	return nil
}
