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

// TestStartInteractive_SignaledShellExitsWith128PlusSignal verifies that `atmos auth
// shell` reports a shell terminated by a signal as 128+N, like `auth exec` and a POSIX
// shell, instead of -1, which the process would report as exit status 255.
func TestStartInteractive_SignaledShellExitsWith128PlusSignal(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	for name, signal := range map[string]syscall.Signal{"SIGKILL": syscall.SIGKILL, "SIGTERM": syscall.SIGTERM} {
		t.Run(name, func(t *testing.T) {
			err := StartInteractive(exe, nil, []string{killSelfEnv + "=" + strconv.Itoa(int(signal))})
			require.Error(t, err)
			var exitErr errUtils.ExitCodeError
			require.ErrorAs(t, err, &exitErr)
			assert.Equal(t, 128+int(signal), exitErr.Code)
			assert.Equal(t, 128+int(signal), errUtils.GetExitCode(err))
		})
	}
}
