package io

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrefixedWriterPrefixesEachLine(t *testing.T) {
	var buf bytes.Buffer
	w := NewPrefixedWriter("stack/component", &buf)

	n, err := w.Write([]byte("one\ntwo\n"))
	require.NoError(t, err)
	assert.Equal(t, len("one\ntwo\n"), n)
	assert.Equal(t, "[stack/component] one\n[stack/component] two\n", buf.String())
}

func TestPrefixedWriterHandlesPartialLines(t *testing.T) {
	var buf bytes.Buffer
	w := NewPrefixedWriter("node", &buf)

	_, err := w.Write([]byte("one"))
	require.NoError(t, err)
	_, err = w.Write([]byte(" two\nthree"))
	require.NoError(t, err)

	assert.Equal(t, "[node] one two\n[node] three", buf.String())
}

func TestPrefixedWriterDiscardsNilWriter(t *testing.T) {
	w := NewPrefixedWriter("node", nil)

	n, err := w.Write([]byte("hello\n"))
	require.NoError(t, err)
	assert.Equal(t, len("hello\n"), n)
}

func TestNewOutputWritesTerminalFileAndCaptureSinks(t *testing.T) {
	var terminal, file, capture bytes.Buffer
	output := NewOutput(OutputOptions{
		Prefix: "component-a",
		Stdout: OutputSinks{
			Terminal: &terminal,
			File:     &file,
			Capture:  &capture,
		},
	})

	_, err := output.Stdout.Write([]byte("hello\n"))
	require.NoError(t, err)

	expected := "[component-a] hello\n"
	assert.Equal(t, expected, terminal.String())
	assert.Equal(t, expected, file.String())
	assert.Equal(t, expected, capture.String())
}

func TestNewOutputMasksAllSinks(t *testing.T) {
	// Under -shuffle=on, another test in this package may have left the
	// shared viper "mask" key or globalContext in a non-default state (e.g.
	// TestNewContextWithMaskingDisabled sets mask=false); reset both so this
	// test's masking behavior doesn't depend on run order.
	resetGlobals()
	t.Cleanup(resetGlobals)

	require.NoError(t, Initialize())
	RegisterSecret("secret-value")

	var terminal, file, capture bytes.Buffer
	output := NewOutput(OutputOptions{
		Prefix: "component-a",
		Stdout: OutputSinks{
			Terminal: &terminal,
			File:     &file,
			Capture:  &capture,
		},
	})

	_, err := output.Stdout.Write([]byte("secret-value\n"))
	require.NoError(t, err)

	assert.NotContains(t, terminal.String(), "secret-value")
	assert.NotContains(t, file.String(), "secret-value")
	assert.NotContains(t, capture.String(), "secret-value")
	assert.Contains(t, terminal.String(), MaskReplacement)
	assert.Contains(t, file.String(), MaskReplacement)
	assert.Contains(t, capture.String(), MaskReplacement)
}
