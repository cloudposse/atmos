package asciicast

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

func TestFinishSessionPreservesDeadlineWhenBothReady(t *testing.T) {
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	<-ctx.Done()
	readErr := errors.New("read failed")
	waitErr := errors.New("process failed")
	completed := false
	// Both channels are ready before every select, without timer or goroutine
	// scheduling. Check either outcome and require coverage of the done branch.
	for range 128 {
		done := make(chan error, 1)
		done <- readErr
		killed := false
		proc := &sessionProcess{
			input: &closeTrackingWriter{Writer: io.Discard},
			kill:  func() { killed = true },
			wait:  func() error { return waitErr },
		}
		err := finishSession(ctx, proc, done)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.ErrorIs(t, err, waitErr)
		if !killed {
			completed = true
			require.ErrorIs(t, err, readErr)
		}
	}
	require.True(t, completed, "must exercise completion with the deadline already exceeded")
}

func TestFinishSessionPreservesDeadlineDuringProcessWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		done := make(chan error, 1)
		done <- nil
		proc := &sessionProcess{
			input: &closeTrackingWriter{Writer: io.Discard},
			kill:  func() { t.Error("completion must be selected before the deadline") },
			wait: func() error {
				<-ctx.Done()
				return nil
			},
		}
		// Only done is ready at selection. Virtual time advances to the deadline
		// while reaping, deterministically exercising cancellation on completion.
		require.ErrorIs(t, finishSession(ctx, proc, done), context.DeadlineExceeded)
	})
}

// cancelAfterFirstWrite cancels the context as soon as the first byte is written,
// so the pacing sleep that follows must observe the cancellation.
type cancelAfterFirstWrite struct {
	buf    bytes.Buffer
	cancel context.CancelFunc
}

func (w *cancelAfterFirstWrite) Write(p []byte) (int, error) {
	n, err := w.buf.Write(p)
	w.cancel()
	return n, err
}

func TestTypedWriteStopsWhenCancelledDuringPacing(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	sink := &cancelAfterFirstWrite{cancel: cancel}
	start := time.Now()
	err := runWriteAction(ctx, &syncWriter{w: sink}, &SessionAction{Text: "abc", Rate: "1h"}, 0)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, "a", sink.buf.String(), "typing must stop after the first rune once cancelled")
	require.Less(t, time.Since(start), 30*time.Second, "an hour-long pacing delay must be interruptible")
}

func TestRepeatedKeyStopsWhenCancelledDuringInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	sink := &cancelAfterFirstWrite{cancel: cancel}
	err := runKeyAction(ctx, &syncWriter{w: sink}, &SessionAction{Key: "enter", Repeat: 3, Interval: "1h"}, 0)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, "\r", sink.buf.String(), "no further repeats may be sent after cancellation")
}

func TestRepeatedKeyStopsWhenAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var sink bytes.Buffer
	err := runKeyAction(ctx, &syncWriter{w: &sink}, &SessionAction{Key: "enter", Repeat: 2}, 0)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, sink.String())
}

func TestSleepSessionActionHonorsContext(t *testing.T) {
	require.NoError(t, sleepSessionAction(t.Context(), time.Nanosecond))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, sleepSessionAction(ctx, time.Hour), context.Canceled)
}

func TestSessionEchoNotesAreNilSafe(t *testing.T) {
	var state *sessionState
	require.NotPanics(t, func() {
		state.noteTypedWrite("echo hi")
		state.noteKeyPress("enter")
		state.noteSubmittedLine()
	})
}

func TestRunSessionObserveFailureStopsTheSession(t *testing.T) {
	require.NoError(t, iolib.Initialize())
	shell, err := os.Executable()
	require.NoError(t, err)
	t.Setenv(asciicastSessionHelperEnv, "1")

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	errObserve := errors.New("checkpoint rejected")
	var observed []int
	err = RunSession(ctx, &SessionOptions{
		Shell: shell,
		Actions: []SessionAction{
			{Type: "pause", Duration: "1ms"},
			{Type: "pause", Duration: "1ms"},
			{Type: "pause", Duration: "1ms"},
		},
		Observe: func(index int, _ *SessionAction) error {
			observed = append(observed, index)
			if index == 1 {
				return errObserve
			}
			return nil
		},
	})
	require.ErrorIs(t, err, errObserve)
	require.Equal(t, []int{0, 1}, observed, "actions after the failed checkpoint must not run")
}

func TestRunSessionBeforeActionsFailureStopsSession(t *testing.T) {
	require.NoError(t, iolib.Initialize())
	shell, err := os.Executable()
	require.NoError(t, err)
	t.Setenv(asciicastSessionHelperEnv, "1")
	failure := errors.New("readiness rejected")
	observed := false
	err = RunSession(t.Context(), &SessionOptions{
		Shell:         shell,
		BeforeActions: func(context.Context, func(string) bool) error { return failure },
		Actions:       []SessionAction{{Type: "pause", Duration: "1ms"}},
		Observe:       func(int, *SessionAction) error { observed = true; return nil },
	})
	require.ErrorIs(t, err, failure)
	require.False(t, observed)
}
