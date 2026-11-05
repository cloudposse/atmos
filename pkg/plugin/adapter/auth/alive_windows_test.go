//go:build windows

package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"

	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/plugin/testkit/fakeplugin"
)

func processAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return !errors.Is(err, windows.ERROR_INVALID_PARAMETER)
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	// An exited process may retain open handles; its signal state determines liveness.
	event, err := windows.WaitForSingleObject(handle, 0)
	return err != nil || event != windows.WAIT_OBJECT_0
}

func TestProcessAliveWithRetainedHandle(t *testing.T) {
	definition, directory := fakeDefinition(t, fakeplugin.ModeHang)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- definition.invoke(ctx, "corporate", "realm", nil, func(operation *invocation) error {
			return operation.handle.Command(ctx, &pluginv1.CommandRequest{Invocation: operation.request, Name: "hang"}, discard{}, discard{})
		})
	}()
	require.Eventually(t, func() bool { pids, err := fakeplugin.PIDs(directory); return err == nil && len(pids) > 0 }, 20*time.Second, 20*time.Millisecond)
	pids, err := fakeplugin.PIDs(directory)
	require.NoError(t, err)
	retained, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pids[0]))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, windows.CloseHandle(retained)) })
	require.True(t, processAlive(pids[0]))
	cancel()
	select {
	case err := <-result:
		require.Error(t, err)
	case <-time.After(20 * time.Second):
		t.Fatal("cancelled invocation did not return")
	}
	require.Eventually(t, func() bool { return !processAlive(pids[0]) }, 20*time.Second, 20*time.Millisecond, "retained handles must not make an exited plugin appear alive")
}
