package native

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/testkit/fakeplugin"
)

// captureWarnings records every warning the host shows the user for the rest of the test.
func captureWarnings(t *testing.T) func() []string {
	t.Helper()
	var mu sync.Mutex
	var shown []string
	previous := warn
	warn = func(text string) {
		mu.Lock()
		defer mu.Unlock()
		shown = append(shown, text)
	}
	t.Cleanup(func() { warn = previous })
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), shown...)
	}
}

// awaitResult waits for a running invocation to return after it was cancelled.
func awaitResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(eventually):
		t.Fatal("Invoke must return after cancellation")
		return nil
	}
}

func TestCancelledInvocationTellsUserPluginLogsMayBeOmitted(t *testing.T) {
	t.Run("interrupt", func(t *testing.T) {
		shown := captureWarnings(t)
		deliver := injectSignals(t)
		handle, directory := startFake(t, t.Context(), fakeplugin.ModeHang)
		_, result := runHanging(t, handle, directory, t.Context())
		deliver(os.Interrupt)
		require.ErrorIs(t, awaitResult(t, result), context.Canceled)
		require.Equal(t, []string{"Interrupted; the plugin process was stopped.", omittedLogsNotice}, shown())
	})
	t.Run("call timeout", func(t *testing.T) {
		shown := captureWarnings(t)
		handle, directory := startFake(t, t.Context(), fakeplugin.ModeHang, WithCallTimeout(500*time.Millisecond))
		_, result := runHanging(t, handle, directory, t.Context())
		require.ErrorIs(t, awaitResult(t, result), errUtils.ErrPluginTimeout)
		require.Equal(t, []string{omittedLogsNotice}, shown())
	})
	t.Run("parent context", func(t *testing.T) {
		shown := captureWarnings(t)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		handle, directory := startFake(t, t.Context(), fakeplugin.ModeHang)
		_, result := runHanging(t, handle, directory, ctx)
		cancel()
		err := awaitResult(t, result)
		require.Error(t, err)
		require.NotErrorIs(t, err, errUtils.ErrPluginTimeout, "a caller cancellation is not a timeout")
		require.Equal(t, []string{omittedLogsNotice}, shown())
	})
}

func TestOmittedLogsNoticeOnlyFollowsCancellation(t *testing.T) {
	shown := captureWarnings(t)
	handle := &Handle{}
	failure := errors.New("plugin failed")

	require.NoError(t, handle.finishInvocation(t.Context(), nil))
	require.ErrorIs(t, handle.finishInvocation(t.Context(), failure), failure)
	require.Empty(t, shown(), "a completed or plainly failed call has delivered its logs in the response")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, handle.finishInvocation(ctx, failure), failure, "a caller cancellation keeps the original error")
	require.Equal(t, []string{omittedLogsNotice}, shown())
}
