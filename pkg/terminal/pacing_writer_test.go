package terminal

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsSpeedLimited(t *testing.T) {
	assert.False(t, IsSpeedLimited(0))
	assert.False(t, IsSpeedLimited(-1))
	assert.True(t, IsSpeedLimited(8))
}

// erroringWriter always fails, used to exercise PacingWriter's error paths.
type erroringWriter struct{}

var errPacingWrite = errors.New("pacing write failed")

func (erroringWriter) Write(p []byte) (int, error) {
	return 0, errPacingWrite
}

func TestPacingWriter_NilReceiverAndWriter(t *testing.T) {
	var nilWriter *PacingWriter
	n, err := nilWriter.Write([]byte("data"))
	require.NoError(t, err)
	assert.Equal(t, len("data"), n)
	require.NoError(t, nilWriter.Close())

	empty := &PacingWriter{}
	n, err = empty.Write([]byte("data"))
	require.NoError(t, err)
	assert.Equal(t, len("data"), n)
	require.NoError(t, empty.Close())
}

func TestPacingWriter_WriteEmptyInput(t *testing.T) {
	var out bytes.Buffer
	writer := NewPacingWriter(&out, 10)
	n, err := writer.Write(nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestPacingWriter_WritePropagatesUnderlyingError(t *testing.T) {
	writer := NewPacingWriter(erroringWriter{}, 10)
	writer.sleep = func(time.Duration) {}

	_, err := writer.Write([]byte("line\n"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errPacingWrite)
}

func TestPacingWriter_ClosePropagatesUnderlyingError(t *testing.T) {
	writer := NewPacingWriter(erroringWriter{}, 0)
	// Buffer a partial line directly (speed=0 means Write won't try to flush via sleep).
	writer.buffer.WriteString("partial")

	err := writer.Close()
	require.Error(t, err)
	assert.ErrorIs(t, err, errPacingWrite)
}

func TestIsTTYWriter(t *testing.T) {
	t.Run("non-file writer returns false", func(t *testing.T) {
		var buf bytes.Buffer
		assert.False(t, IsTTYWriter(&buf))
	})

	t.Run("nil writer returns false", func(t *testing.T) {
		assert.False(t, IsTTYWriter(nil))
	})
}

func TestHasRealTTYInput(t *testing.T) {
	// In CI/test environments stdin is typically not a real terminal, so this
	// should return false. We just verify it doesn't panic and returns a bool.
	_ = HasRealTTYInput()
}

func TestPacingWriterWritesCompleteLinesAtSpeed(t *testing.T) {
	var out bytes.Buffer
	writer := NewPacingWriter(&out, 10)

	var sleeps []time.Duration
	writer.sleep = func(d time.Duration) {
		sleeps = append(sleeps, d)
	}

	n, err := writer.Write([]byte("one\ntwo\npartial"))
	require.NoError(t, err)
	assert.Equal(t, len("one\ntwo\npartial"), n)
	assert.Equal(t, "one\ntwo\n", out.String())
	assert.Equal(t, []time.Duration{100 * time.Millisecond, 100 * time.Millisecond}, sleeps)

	require.NoError(t, writer.Close())
	assert.Equal(t, "one\ntwo\npartial", out.String())
}

func TestPacingWriterBuffersPartialLinesAcrossWrites(t *testing.T) {
	var out bytes.Buffer
	writer := NewPacingWriter(&out, 20)
	writer.sleep = func(time.Duration) {}

	_, err := writer.Write([]byte("one"))
	require.NoError(t, err)
	assert.Empty(t, out.String())

	_, err = writer.Write([]byte(" two\n"))
	require.NoError(t, err)
	assert.Equal(t, "one two\n", out.String())
}

func TestPacingWriterCloseFlushesPartialLineImmediately(t *testing.T) {
	var out bytes.Buffer
	writer := NewPacingWriter(&out, 1)

	var slept bool
	writer.sleep = func(time.Duration) {
		slept = true
	}

	_, err := writer.Write([]byte("partial"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	assert.Equal(t, "partial", out.String())
	assert.False(t, slept)
}
