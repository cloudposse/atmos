package asciicast

import (
	"errors"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

// closeFlagWriter records whether Close was called, to prove syncWriter
// forwards Close to writers that support it.
type closeFlagWriter struct {
	closed   bool
	closeErr error
}

func (w *closeFlagWriter) Write(p []byte) (int, error) { return len(p), nil }

func (w *closeFlagWriter) Close() error {
	w.closed = true
	return w.closeErr
}

func TestSyncWriterClose(t *testing.T) {
	closeErr := errors.New("close failed")
	tests := []struct {
		name       string
		writer     io.Writer
		wantErr    error
		wantClosed bool
	}{
		{name: "forwards Close to an io.Closer", writer: &closeFlagWriter{}, wantClosed: true},
		{name: "propagates the underlying Close error", writer: &closeFlagWriter{closeErr: closeErr}, wantErr: closeErr, wantClosed: true},
		{name: "no-op for a writer that cannot close", writer: io.Discard},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&syncWriter{w: tt.writer}).Close()
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tracker, ok := tt.writer.(*closeFlagWriter); ok {
				assert.Equal(t, tt.wantClosed, tracker.closed)
			}
		})
	}
}

func TestSessionEnvironment(t *testing.T) {
	t.Setenv("ATMOS_SESSION_ENV_TEST_BASE", "base")

	t.Run("empty overrides return the process environment", func(t *testing.T) {
		got := sessionEnvironment(nil)
		assert.Equal(t, os.Environ(), got)
	})

	t.Run("overrides replace inherited values and add new ones", func(t *testing.T) {
		got := sessionEnvironment(map[string]string{
			"ATMOS_SESSION_ENV_TEST_BASE": "overridden",
			"ATMOS_SESSION_ENV_TEST_NEW":  "added",
		})
		assert.Contains(t, got, "ATMOS_SESSION_ENV_TEST_BASE=overridden")
		assert.Contains(t, got, "ATMOS_SESSION_ENV_TEST_NEW=added")
		assert.NotContains(t, got, "ATMOS_SESSION_ENV_TEST_BASE=base")
	})
}

func TestSetDiscardTogglesRecordingAndIsNoopOnNilState(t *testing.T) {
	var nilState *sessionState
	nilState.setDiscard(true) // Must not panic.

	state := &sessionState{}
	state.setDiscard(true)
	assert.True(t, state.discardRecording)
	state.setDiscard(false)
	assert.False(t, state.discardRecording)
}

func TestWaitForSessionProcessReturnsNilWithoutProcessOrWait(t *testing.T) {
	require.NoError(t, waitForSessionProcess(nil, time.Second))
	require.NoError(t, waitForSessionProcess(&sessionProcess{}, time.Second))
}

func TestRunCallbackAction(t *testing.T) {
	fnErr := errors.New("callback failed")
	tests := []struct {
		name      string
		action    SessionAction
		wantErr   error
		wantCalls int
	}{
		{name: "simulate without callback", action: SessionAction{Type: "simulate"}, wantErr: ErrSimulateActionMissingCallback},
		{name: "simulate runs callback once", action: SessionAction{Type: "simulate", Fn: func() error { return nil }}, wantCalls: 1},
		{name: "simulate propagates callback error", action: SessionAction{Type: "simulate", Fn: func() error { return fnErr }}, wantErr: fnErr, wantCalls: 1},
		{name: "unknown type", action: SessionAction{Type: "bogus"}, wantErr: ErrUnknownSessionAction},
		{name: "screenshot without path", action: SessionAction{Type: "screenshot"}, wantErr: ErrScreenshotActionRequiresPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			if tt.action.Fn != nil {
				inner := tt.action.Fn
				tt.action.Fn = func() error {
					calls++
					return inner()
				}
			}
			err := runCallbackAction(&tt.action)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantCalls, calls)
		})
	}
}

// markerCollector captures every event passed to the process-wide recorder.
type markerCollector struct {
	mu     sync.Mutex
	events [][2]string
}

func (c *markerCollector) Record(stream, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, [2]string{stream, content})
}

func TestRunScreenshotActionRecordsMarker(t *testing.T) {
	collector := &markerCollector{}
	restore := iolib.SetRecorder(collector)
	defer restore()

	require.NoError(t, runCallbackAction(&SessionAction{Type: "screenshot", Path: "after.png"}))

	collector.mu.Lock()
	defer collector.mu.Unlock()
	require.Len(t, collector.events, 1)
	assert.Equal(t, [2]string{"m", "after.png"}, collector.events[0])
}

func TestRunPlaybackActionRejectsInvalidShape(t *testing.T) {
	nested := []SessionAction{{Type: "pause", Duration: "1ms"}}
	tests := []struct {
		name   string
		action SessionAction
	}{
		{name: "unparsable duration", action: SessionAction{Type: "playback", Duration: "soon", Actions: nested}},
		{name: "zero duration", action: SessionAction{Type: "playback", Duration: "0s", Actions: nested}},
		{name: "no nested actions", action: SessionAction{Type: "playback", Duration: "1s"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runPlaybackAction(t.Context(), &syncWriter{w: io.Discard}, &sessionState{}, &tt.action, &SessionOptions{})
			require.ErrorIs(t, err, errUtils.ErrInvalidCastPlayback)
			assert.Contains(t, err.Error(), "positive duration")
		})
	}
}
