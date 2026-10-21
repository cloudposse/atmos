//go:build windows

package native

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"

	"github.com/cloudposse/atmos/pkg/plugin/testkit/fakeplugin"
)

func processAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return !errors.Is(err, windows.ERROR_INVALID_PARAMETER)
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	// A retained handle can keep an exited process open; only its signal state
	// tells us whether it is still running.
	event, err := windows.WaitForSingleObject(handle, 0)
	return err != nil || event != windows.WAIT_OBJECT_0
}

func TestProcessAliveWithRetainedHandle(t *testing.T) {
	handle, directory := startFake(t, t.Context(), fakeplugin.ModeHang)
	pids, result := runHanging(t, handle, directory, t.Context())
	require.Len(t, pids, 1)
	retained, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pids[0]))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, windows.CloseHandle(retained)) })
	require.True(t, processAlive(pids[0]))

	handle.Close()
	<-result
	require.False(t, processAlive(pids[0]), "an exited process is dead even while its handle remains open")
}
