package terminal

import (
	"bytes"
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
