package auth

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/keyring"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/plugin/testkit/fakeplugin"
)

type discard struct{}

func (discard) Write(value []byte) (int, error) { return len(value), nil }

func fakeDefinition(t *testing.T, mode string) (*Definition, string) {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	directory := t.TempDir()
	require.NoError(t, fakeplugin.WriteMode(directory, mode))
	storage, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	return &Definition{Package: "example/fake", Executable: executable, Directory: directory, Services: Services{Keyring: storage, LockDirectory: t.TempDir()}}, directory
}

func TestCancelledInvocationKillsPluginAndReleasesSessionLock(t *testing.T) {
	definition, directory := fakeDefinition(t, fakeplugin.ModeHang)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- definition.invoke(ctx, "corporate", "realm", nil, func(operation *invocation) error {
			return operation.handle.Command(ctx, &pluginv1.CommandRequest{Invocation: operation.request, Name: "hang"}, discard{}, discard{})
		})
	}()
	require.Eventually(t, func() bool {
		pids, err := fakeplugin.PIDs(directory)
		return err == nil && len(pids) > 0
	}, 20*time.Second, 20*time.Millisecond)
	pids, err := fakeplugin.PIDs(directory)
	require.NoError(t, err)
	require.True(t, processAlive(pids[0]))

	cancel()
	select {
	case err := <-result:
		require.Error(t, err)
	case <-time.After(20 * time.Second):
		t.Fatal("a cancelled invocation must return")
	}
	require.Eventually(t, func() bool { return !processAlive(pids[0]) }, 20*time.Second, 20*time.Millisecond, "the plugin must not outlive cancellation")

	namespace, err := definition.sessionNamespace("corporate", "realm", nil)
	require.NoError(t, err)
	lockCtx, stop := context.WithTimeout(t.Context(), 10*time.Second)
	defer stop()
	acquired := false
	require.NoError(t, definition.transaction(lockCtx, namespace, func(context.Context) error { acquired = true; return nil }))
	require.True(t, acquired, "the session lock must be released after cancellation")
}

func TestInvocationDeadlineSurfacesPluginTimeoutThroughAuthTranslation(t *testing.T) {
	definition, directory := fakeDefinition(t, fakeplugin.ModeHang)
	// Bound the invocation after startup so a slow race-instrumented handshake
	// cannot prevent the test from reaching the auth error translation boundary.
	definition.Timeout = 500 * time.Millisecond
	err := definition.invoke(t.Context(), "corporate", "realm", nil, func(operation *invocation) error {
		require.NotNil(t, operation.request.Deadline)
		ctx, cancel := context.WithDeadline(t.Context(), operation.request.Deadline.AsTime())
		defer cancel()
		return operation.handle.Command(ctx, &pluginv1.CommandRequest{Invocation: operation.request, Name: "hang"}, discard{}, discard{})
	})
	require.ErrorIs(t, err, errUtils.ErrPluginTimeout)
	require.NotEmpty(t, errUtils.AllHints(err))
	pids, pidErr := fakeplugin.PIDs(directory)
	require.NoError(t, pidErr)
	require.NotEmpty(t, pids, "the test must reach the running plugin command")
	require.Eventually(t, func() bool { return !processAlive(pids[0]) }, 20*time.Second, 20*time.Millisecond)
}

func TestCompletedInvocationIsNotReportedAsTimeout(t *testing.T) {
	definition, _ := fakeDefinition(t, fakeplugin.ModeEcho)
	err := definition.invoke(t.Context(), "corporate", "realm", nil, func(operation *invocation) error {
		return operation.handle.Command(t.Context(), &pluginv1.CommandRequest{Invocation: operation.request, Name: "echo"}, discard{}, discard{})
	})
	require.NoError(t, err)
}
