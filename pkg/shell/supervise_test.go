package shell

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestForwardSignal_FinishedProcess verifies that forwarding a signal to a child that
// already exited is harmless (the error is only logged).
func TestForwardSignal_FinishedProcess(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "_ATMOS_SHELL_TEST_EXIT_OK=1")
	require.NoError(t, cmd.Run())

	require.NotPanics(t, func() { forwardSignal(cmd.Process, os.Kill) })
}
