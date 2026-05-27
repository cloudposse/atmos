package io

import (
	"bytes"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLinePrefixWriterBuffersPartialLines(t *testing.T) {
	var out bytes.Buffer
	writer := NewLinePrefixWriter("stack/component", &out, &sync.Mutex{})

	_, err := writer.Write([]byte("hello"))
	require.NoError(t, err)
	require.Empty(t, out.String())

	_, err = writer.Write([]byte(" world\nnext"))
	require.NoError(t, err)
	require.Equal(t, "[stack/component] hello world\n", out.String())

	require.NoError(t, writer.Flush())
	require.Equal(t, "[stack/component] hello world\n[stack/component] next", out.String())
}

func TestLinePrefixWriterSerializesCompleteLines(t *testing.T) {
	var out bytes.Buffer
	writeMu := &sync.Mutex{}
	first := NewLinePrefixWriter("first", &out, writeMu)
	second := NewLinePrefixWriter("second", &out, writeMu)

	_, err := first.Write([]byte("a\n"))
	require.NoError(t, err)
	_, err = second.Write([]byte("b\n"))
	require.NoError(t, err)

	require.Equal(t, "[first] a\n[second] b\n", out.String())
}

func TestLinePrefixWriterWithoutPrefixWritesRawLines(t *testing.T) {
	var out bytes.Buffer
	writer := NewLinePrefixWriter("", &out, nil)

	n, err := writer.Write([]byte("raw\n"))
	require.NoError(t, err)
	require.Equal(t, 4, n)
	require.Equal(t, "raw\n", out.String())

	require.NoError(t, writer.Flush())
	require.Equal(t, "raw\n", out.String())
}

func TestLinePrefixWriterEmptyWriteAndFlushAreNoops(t *testing.T) {
	var out bytes.Buffer
	writer := NewLinePrefixWriter("node", &out, nil)

	n, err := writer.Write(nil)
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.NoError(t, writer.Flush())
	require.Empty(t, out.String())
}

func TestLinePrefixWriterNilTargetDropsOutput(t *testing.T) {
	writer := NewLinePrefixWriter("node", nil, nil)

	n, err := writer.Write([]byte("line\n"))
	require.NoError(t, err)
	require.Equal(t, len("line\n"), n)
	require.NoError(t, writer.Flush())
}

func TestLinePrefixWriterPrefixesCarriageReturnSegments(t *testing.T) {
	var out bytes.Buffer
	writer := NewLinePrefixWriter("node", &out, nil)

	_, err := writer.Write([]byte("first\rsecond\n"))
	require.NoError(t, err)
	require.Equal(t, "[node] first\r[node] second\n", out.String())
}

func TestLinePrefixWriterPropagatesWriteErrors(t *testing.T) {
	expectedErr := errors.New("write failed")
	writer := NewLinePrefixWriter("node", linePrefixErrorWriter{err: expectedErr}, nil)

	n, err := writer.Write([]byte("line\n"))
	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, 0, n)

	writer = NewLinePrefixWriter("node", linePrefixErrorWriter{err: expectedErr}, nil)
	_, err = writer.Write([]byte("partial"))
	require.NoError(t, err)
	require.ErrorIs(t, writer.Flush(), expectedErr)
}

func TestLinePrefixWriterKeepsBufferedOutputAfterWriteErrors(t *testing.T) {
	expectedErr := errors.New("write failed")
	target := &linePrefixFailOnceWriter{err: expectedErr}
	writer := NewLinePrefixWriter("node", target, nil)

	n, err := writer.Write([]byte("line\n"))
	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, 0, n)
	require.Empty(t, target.out.String())

	require.NoError(t, writer.Flush())
	require.Equal(t, "[node] line\n", target.out.String())

	target = &linePrefixFailOnceWriter{err: expectedErr}
	writer = NewLinePrefixWriter("node", target, nil)
	_, err = writer.Write([]byte("partial"))
	require.NoError(t, err)
	require.ErrorIs(t, writer.Flush(), expectedErr)
	require.Empty(t, target.out.String())

	require.NoError(t, writer.Flush())
	require.Equal(t, "[node] partial", target.out.String())
}

type linePrefixErrorWriter struct {
	err error
}

func (w linePrefixErrorWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}

type linePrefixFailOnceWriter struct {
	err    error
	failed bool
	out    bytes.Buffer
}

func (w *linePrefixFailOnceWriter) Write(p []byte) (int, error) {
	if !w.failed {
		w.failed = true
		return 0, w.err
	}
	return w.out.Write(p)
}
