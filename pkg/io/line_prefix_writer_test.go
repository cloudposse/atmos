package io

import (
	"bytes"
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
