package native

import (
	"context"
	"errors"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/bridge"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/plugin/testkit/fakeplugin"
	"github.com/cloudposse/atmos/pkg/signals"
)

const eventually = 20 * time.Second

// startFake launches the test binary as a plugin in the given mode and returns its handle.
func startFake(t *testing.T, ctx context.Context, mode string, opts ...Option) (*Handle, string) {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	directory := t.TempDir()
	require.NoError(t, fakeplugin.WriteMode(directory, mode))
	handle, err := Start(ctx, executable, append([]Option{WithDirectory(directory)}, opts...)...)
	require.NoError(t, err)
	t.Cleanup(handle.Close)
	return handle, directory
}

// runHanging starts a command that never returns and reports the recorded process ids.
func runHanging(t *testing.T, handle *Handle, directory string, ctx context.Context) (pids []int, result <-chan error) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		done <- handle.Invoke(ctx, bridge.Invocation{Namespace: "test"}, func(ctx context.Context, invocation *pluginv1.Invocation) error {
			return handle.Command(ctx, &pluginv1.CommandRequest{Invocation: invocation, Name: "hang"}, nopWriter{}, nopWriter{})
		})
	}()
	require.Eventually(t, func() bool {
		recorded, err := fakeplugin.PIDs(directory)
		return err == nil && len(recorded) > 0
	}, eventually, 20*time.Millisecond, "the plugin must start its long-running command")
	pids, err := fakeplugin.PIDs(directory)
	require.NoError(t, err)
	for _, pid := range pids {
		require.True(t, processAlive(pid), "pid %d must be running before cancellation", pid)
	}
	return pids, done
}

type nopWriter struct{}

func (nopWriter) Write(value []byte) (int, error) { return len(value), nil }

func requireGone(t *testing.T, pids []int) {
	t.Helper()
	for _, pid := range pids {
		require.Eventually(t, func() bool { return !processAlive(pid) }, eventually, 20*time.Millisecond, "pid %d must be gone", pid)
	}
}

func TestCancellingContextKillsPluginProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	handle, directory := startFake(t, ctx, fakeplugin.ModeHang)
	pids, result := runHanging(t, handle, directory, ctx)
	cancel()
	select {
	case err := <-result:
		require.Error(t, err)
	case <-time.After(eventually):
		t.Fatal("Invoke must return after cancellation")
	}
	requireGone(t, pids)
}

func TestCloseKillsPluginAndItsChildren(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no process group to signal.")
	}
	handle, directory := startFake(t, t.Context(), fakeplugin.ModeSpawn)
	pids, result := runHanging(t, handle, directory, t.Context())
	require.Len(t, pids, 2, "the plugin and the process it spawned")
	handle.Close()
	requireGone(t, pids)
	<-result
}

// injectSignals replaces OS signal registration so tests deliver signals to Start's subscription.
func injectSignals(t *testing.T) func(os.Signal) {
	t.Helper()
	var delivered chan<- os.Signal
	previousNotify, previousReset := notifySignals, resetSignals
	notifySignals = func(ch chan<- os.Signal, _ ...os.Signal) { delivered = ch }
	resetSignals = func(chan<- os.Signal) {}
	t.Cleanup(func() { notifySignals, resetSignals = previousNotify, previousReset })
	return func(sig os.Signal) { delivered <- sig }
}

func TestInterruptSignalStopsPluginBeforeReturning(t *testing.T) {
	for name, tc := range map[string]struct {
		signal os.Signal
		code   int
	}{"SIGINT": {os.Interrupt, 130}, "SIGTERM": {syscall.SIGTERM, 143}} {
		t.Run(name, func(t *testing.T) {
			deliver := injectSignals(t)
			handle, directory := startFake(t, t.Context(), fakeplugin.ModeHang)
			pids, result := runHanging(t, handle, directory, t.Context())
			deliver(tc.signal)
			select {
			case err := <-result:
				require.ErrorIs(t, err, context.Canceled)
				var exit errUtils.ExitCodeError
				require.ErrorAs(t, err, &exit)
				require.Equal(t, tc.code, exit.Code, "exit status follows the received signal")
				requireGone(t, pids)
			case <-time.After(eventually):
				t.Fatal("Invoke must return after an interrupt")
			}
		})
	}
}

func TestStartSubscribesToInterruptAndTerminate(t *testing.T) {
	var subscribed []os.Signal
	previous := notifySignals
	notifySignals = func(_ chan<- os.Signal, signals ...os.Signal) { subscribed = signals }
	t.Cleanup(func() { notifySignals = previous })
	startFake(t, t.Context(), fakeplugin.ModeEcho)
	require.ElementsMatch(t, []os.Signal{os.Interrupt, syscall.SIGTERM}, subscribed)
}

func TestExitCleanupStopsPluginWhenProcessHandlerExits(t *testing.T) {
	handle, directory := startFake(t, t.Context(), fakeplugin.ModeHang)
	pids, result := runHanging(t, handle, directory, t.Context())
	// main's signal handler calls this and then exits without unwinding any defers.
	signals.RunExitCleanups()
	requireGone(t, pids)
	<-result
}

func TestNoExitCleanupRemainsAfterClose(t *testing.T) {
	handle, directory := startFake(t, t.Context(), fakeplugin.ModeHang)
	pids, result := runHanging(t, handle, directory, t.Context())
	handle.Close()
	requireGone(t, pids)
	<-result
	signals.RunExitCleanups() // Must not touch the closed handle again.
}

func TestCallTimeoutStopsPluginAndReportsDeadline(t *testing.T) {
	handle, directory := startFake(t, t.Context(), fakeplugin.ModeHang, WithCallTimeout(500*time.Millisecond))
	pids, result := runHanging(t, handle, directory, t.Context())
	select {
	case err := <-result:
		require.ErrorIs(t, err, errUtils.ErrPluginTimeout)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.NotEmpty(t, errUtils.AllHints(err), "the timeout explains what to do")
	case <-time.After(eventually):
		t.Fatal("Invoke must return after the deadline")
	}
	requireGone(t, pids)
}

func TestCallerDeadlineIsReportedAsPluginTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	handle, directory := startFake(t, t.Context(), fakeplugin.ModeHang)
	pids, result := runHanging(t, handle, directory, ctx)
	require.ErrorIs(t, <-result, errUtils.ErrPluginTimeout)
	requireGone(t, pids)
}

func TestInvocationCarriesDeadlineOnlyWhenOneExists(t *testing.T) {
	handle, _ := startFake(t, t.Context(), fakeplugin.ModeEcho)
	capture := func(ctx context.Context, timeout time.Duration) *pluginv1.Invocation {
		handle.callTimeout = timeout
		var seen *pluginv1.Invocation
		require.NoError(t, handle.Invoke(ctx, bridge.Invocation{Namespace: "test"}, func(_ context.Context, invocation *pluginv1.Invocation) error {
			seen = invocation
			return nil
		}))
		return seen
	}
	require.Nil(t, capture(t.Context(), 0).GetDeadline(), "no deadline, no timestamp")

	deadline := time.Now().Add(time.Hour)
	ctx, cancel := context.WithDeadline(t.Context(), deadline)
	defer cancel()
	got := capture(ctx, 0).GetDeadline()
	require.NotNil(t, got)
	require.WithinDuration(t, deadline, got.AsTime(), time.Second)

	got = capture(t.Context(), 30*time.Minute).GetDeadline()
	require.NotNil(t, got, "the call timeout is visible to the plugin")
	require.WithinDuration(t, time.Now().Add(30*time.Minute), got.AsTime(), time.Minute)
}

func TestSuccessfulCommandIsNotInterruptedOrTimedOut(t *testing.T) {
	handle, _ := startFake(t, t.Context(), fakeplugin.ModeEcho, WithCallTimeout(time.Minute))
	err := handle.Invoke(t.Context(), bridge.Invocation{Namespace: "test"}, func(ctx context.Context, invocation *pluginv1.Invocation) error {
		return handle.Command(ctx, &pluginv1.CommandRequest{Invocation: invocation, Name: "echo"}, nopWriter{}, nopWriter{})
	})
	require.NoError(t, err)
	require.False(t, errors.Is(err, errUtils.ErrPluginTimeout))
}
