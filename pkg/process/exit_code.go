package process

import (
	"errors"
	"os/exec"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ExitCode extracts a shell-style exit code from err.
func ExitCode(err error) int {
	defer perf.Track(nil, "process.ExitCode")()

	var exitCodeErr errUtils.ExitCodeError
	if errors.As(err, &exitCodeErr) {
		return exitCodeErr.Code
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ProcessState == nil {
			return exitErr.ExitCode()
		}
		return exitStatusCode(exitErr)
	}
	return 1
}
