//go:build !windows

package shell

import (
	"os"
	"strconv"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestRunCommand_SignaledChildExitsWith128PlusSignal verifies that a child
// terminated by a signal propagates the conventional shell status 128+N
// (for example 137 for SIGKILL, 143 for SIGTERM) instead of -1, which the
// process would report as exit status 255.
func TestRunCommand_SignaledChildExitsWith128PlusSignal(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	for name, signal := range map[string]syscall.Signal{"SIGKILL": syscall.SIGKILL, "SIGTERM": syscall.SIGTERM} {
		t.Run(name, func(t *testing.T) {
			err := RunCommand([]string{exe}, []string{killSelfEnv + "=" + strconv.Itoa(int(signal))})
			require.Error(t, err)
			var exitErr errUtils.ExitCodeError
			require.ErrorAs(t, err, &exitErr)
			assert.Equal(t, 128+int(signal), exitErr.Code)
			assert.Equal(t, 128+int(signal), errUtils.GetExitCode(err))
		})
	}
}
