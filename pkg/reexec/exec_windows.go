//go:build windows

package reexec

import (
	"errors"
	"os"
	"os/exec"
)

// Exec approximates Unix execve semantics on Windows: it spawns the child
// with the same stdio, waits for it to exit, and then calls os.Exit with
// the child's status so the parent does not keep running after the
// "replaced" process finishes. The signature matches syscall.Exec so
// calling code stays platform-neutral; the var binding lets tests
// override it.
var Exec ExecFunc = func(argv0 string, argv []string, envv []string) error {
	args := []string{}
	if len(argv) > 1 {
		args = argv[1:]
	}
	cmd := exec.Command(argv0, args...)
	cmd.Env = envv
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			os.Exit(ee.ExitCode())
		}
		return err
	}
	os.Exit(0)
	// Unreachable, but required for the function signature.
	return nil
}
