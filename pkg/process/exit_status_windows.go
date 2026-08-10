//go:build windows

package process

import (
	"os/exec"
)

// exitStatusCode returns the child's exit code. Windows has no signal-death
// wait status to translate.
func exitStatusCode(exitErr *exec.ExitError) int {
	return exitErr.ExitCode()
}

func exitSignalStatus(_ *exec.ExitError) (bool, int, string) {
	return false, 0, ""
}
