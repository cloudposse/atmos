package process

import (
	"errors"
	"os/exec"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RunManaged starts cmd with platform process-tree management and waits for it.
func RunManaged(cmd *exec.Cmd) error {
	defer perf.Track(nil, "process.RunManaged")()

	if cmd == nil {
		return errUtils.ErrProcessStartFailed
	}

	cleanup, err := prepareManagedCommand(cmd)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	if err := activateManagedCommand(cmd); err != nil {
		killErr := killManagedCommand(cmd)
		waitErr := cmd.Wait()
		return errors.Join(err, killErr, waitErr)
	}

	return cmd.Wait()
}
