package asciicast

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPlaybackDurationPreservesFramesAndSurroundingTiming(t *testing.T) {
	rec, err := Start(&Options{Path: filepath.Join(t.TempDir(), "timing.cast"), RecordIn: true})
	require.NoError(t, err)
	var elapsed time.Duration
	rec.elapsed = func() time.Duration { return elapsed }
	elapsed = time.Second
	require.NoError(t, rec.Event("o", "typed command"))
	elapsed = 2 * time.Second
	require.NoError(t, rec.WithPlaybackDuration(4*time.Second, func() error {
		require.NoError(t, rec.Event("i", "\r"))
		elapsed = 7 * time.Second
		require.NoError(t, rec.Event("o", "frame one"))
		elapsed = 12 * time.Second
		require.NoError(t, rec.Event("r", "80x24"))
		elapsed = 22 * time.Second
		return rec.Event("o", "complete\r\n> ")
	}))
	elapsed = 25 * time.Second
	require.NoError(t, rec.Event("o", "next command"))
	require.NoError(t, rec.Close())
	_, events, err := ReadEvents(rec.Path())
	require.NoError(t, err)
	require.Equal(t, []Event{
		{Time: 1, Stream: "o", Data: "typed command"},
		{Time: 2, Stream: "i", Data: "\r"},
		{Time: 3, Stream: "o", Data: "frame one"},
		{Time: 4, Stream: "r", Data: "80x24"},
		{Time: 6, Stream: "o", Data: "complete\r\n> "},
		{Time: 6, Stream: "o", Data: ""},
		{Time: 9, Stream: "o", Data: "next command"},
	}, events)
}

func TestPlaybackDurationRejectsInvalidAndNestedScopes(t *testing.T) {
	rec, err := Start(&Options{Path: filepath.Join(t.TempDir(), "timing.cast")})
	require.NoError(t, err)
	defer rec.Discard()
	for _, duration := range []time.Duration{0, -time.Second} {
		require.Error(t, rec.WithPlaybackDuration(duration, func() error { t.Fatal("invalid scope ran"); return nil }))
	}
	require.NoError(t, rec.WithPlaybackDuration(time.Second, func() error {
		require.Error(t, rec.WithPlaybackDuration(time.Second, func() error { t.Fatal("nested scope ran"); return nil }))
		return nil
	}))
}

func TestPlaybackDurationRetainsSilenceAndRestoresAfterFailure(t *testing.T) {
	rec, err := Start(&Options{Path: filepath.Join(t.TempDir(), "timing.cast")})
	require.NoError(t, err)
	var elapsed time.Duration
	rec.elapsed = func() time.Duration { return elapsed }
	failure := errors.New("command failed")
	require.ErrorIs(t, rec.WithPlaybackDuration(4*time.Second, func() error {
		elapsed = time.Second
		return failure
	}), failure)
	elapsed = 3 * time.Second
	require.NoError(t, rec.Event("o", "after"))
	require.NoError(t, rec.Close())
	_, events, err := ReadEvents(rec.Path())
	require.NoError(t, err)
	require.Equal(t, []Event{{Time: 4, Stream: "o", Data: ""}, {Time: 6, Stream: "o", Data: "after"}}, events)
}

func TestPlaybackDurationHandlesPacedLinesAndConsecutiveGroups(t *testing.T) {
	rec, err := Start(&Options{Path: filepath.Join(t.TempDir(), "timing.cast"), OutputRate: time.Second})
	require.NoError(t, err)
	var elapsed time.Duration
	rec.elapsed = func() time.Duration { return elapsed }
	require.NoError(t, rec.WithPlaybackDuration(4*time.Second, func() error { return rec.Event("o", "a\nb\nc\n") }))
	elapsed = time.Second
	require.NoError(t, rec.WithPlaybackDuration(2*time.Second, func() error {
		elapsed = 2 * time.Second
		return rec.Event("e", "second")
	}))
	elapsed = 3 * time.Second
	require.NoError(t, rec.Event("o", "after"))
	require.NoError(t, rec.Close())
	_, events, err := ReadEvents(rec.Path())
	require.NoError(t, err)
	require.Equal(t, []Event{
		{Time: 0, Stream: "o", Data: "a\n"},
		{Time: 2, Stream: "o", Data: "b\n"},
		{Time: 4, Stream: "o", Data: "c\n"},
		{Time: 4, Stream: "o", Data: ""},
		{Time: 7, Stream: "e", Data: "second"},
		{Time: 7, Stream: "o", Data: ""},
		{Time: 8, Stream: "o", Data: "after"},
	}, events)
	require.Error(t, rec.WithPlaybackDuration(time.Second, func() error { t.Fatal("closed recorder ran callback"); return nil }))
}

func TestPlaybackDurationRequiresRecorder(t *testing.T) {
	require.Error(t, (*Recorder)(nil).WithPlaybackDuration(time.Second, func() error { t.Fatal("missing recorder ran callback"); return nil }))
}
