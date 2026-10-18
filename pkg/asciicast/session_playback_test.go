package asciicast

import (
	"context"
	"io"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/terminal/query"
)

type blockingPlaybackRecorder struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingPlaybackRecorder) Record(_, _ string) {
	r.once.Do(func() { close(r.entered) })
	<-r.release
}

func TestSessionWaitCannotObserveOutputBeforeRecording(t *testing.T) {
	require.NoError(t, iolib.Initialize())
	rec := &blockingPlaybackRecorder{entered: make(chan struct{}), release: make(chan struct{})}
	restore := iolib.SetRecorder(rec)
	defer restore()
	var release sync.Once
	defer release.Do(func() { close(rec.release) })
	state := &sessionState{responder: query.NewResponder(io.Discard), changed: make(chan struct{}, 1)}
	done := make(chan struct{})
	go func() { state.recordOutputChunk([]byte("complete> ")); close(done) }()
	select {
	case <-rec.entered:
	case <-time.After(time.Second):
		t.Fatal("output did not reach recorder")
	}
	// The wait buffer must remain locked until the final frame is recorded.
	if state.mu.TryLock() {
		state.mu.Unlock()
		t.Fatal("wait could observe output before its frame was recorded")
	}
	release.Do(func() { close(rec.release) })
	<-done
	require.True(t, outputMatches(state, "complete> ", nil))
}

func TestSessionPlaybackKeepsRealTimeout(t *testing.T) {
	rec, err := Start(&Options{Path: filepath.Join(t.TempDir(), "timeout.cast")})
	require.NoError(t, err)
	defer rec.Discard()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	action := &SessionAction{Type: "playback", Duration: "4s", Actions: []SessionAction{{Type: "wait", Text: "missing", Timeout: "1ms"}}}
	state := &sessionState{changed: make(chan struct{}, 1)}
	require.ErrorIs(t, runAction(ctx, io.Discard, state, action, &SessionOptions{Recorder: rec}), ErrWaitTimeout)
	require.NoError(t, rec.WithPlaybackDuration(time.Second, func() error { return nil }))
}
