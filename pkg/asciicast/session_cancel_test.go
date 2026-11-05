package asciicast

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

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
