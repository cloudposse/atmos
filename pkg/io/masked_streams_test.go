package io

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// useGlobalSecret initializes the global context, registers secret, and restores state afterwards.
func useGlobalSecret(t *testing.T, secret string) {
	t.Helper()
	resetGlobals()
	t.Cleanup(resetGlobals)
	require.NoError(t, Initialize())
	RegisterSecret(secret)
}

func TestMaskedStreams_SecretSplitAcrossWritesIsMasked(t *testing.T) {
	const secret = "masked-streams-secret-value"
	useGlobalSecret(t, secret)

	var stdout, stderr bytes.Buffer
	ms := NewMaskedStreams(strings.NewReader(""), &stdout, &stderr)

	// Each stream is split mid-secret, as pipe reads from a subprocess can be.
	_, err := ms.Stdout.Write([]byte("out masked-streams-"))
	require.NoError(t, err)
	_, err = ms.Stdout.Write([]byte("secret-value end\n"))
	require.NoError(t, err)
	_, err = ms.Stderr.Write([]byte("err masked-streams-secret"))
	require.NoError(t, err)
	_, err = ms.Stderr.Write([]byte("-value"))
	require.NoError(t, err)
	require.NoError(t, ms.Flush())

	assert.Equal(t, "out "+MaskReplacement+" end\n", stdout.String())
	assert.Equal(t, "err "+MaskReplacement, stderr.String())
	assert.NotContains(t, stdout.String()+stderr.String(), secret)
}

func TestMaskedStreams_FlushReleasesHeldPrefixUnchanged(t *testing.T) {
	useGlobalSecret(t, "masked-streams-secret-value")

	var stdout bytes.Buffer
	ms := NewMaskedStreams(nil, &stdout, nil)

	// Negative path: a prefix that never completes a secret is held, then released verbatim.
	_, err := ms.Stdout.Write([]byte("masked-streams-"))
	require.NoError(t, err)
	assert.Empty(t, stdout.String(), "a possible secret prefix is held until it is resolved")

	require.NoError(t, ms.Flush())
	assert.Equal(t, "masked-streams-", stdout.String())
}

func TestMaskedStreams_NilTargetsAreSkipped(t *testing.T) {
	useGlobalSecret(t, "masked-streams-secret-value")

	var stdout bytes.Buffer
	ms := NewMaskedStreams(nil, &stdout, nil)
	assert.NotNil(t, ms.Stdout)
	assert.Nil(t, ms.Stderr)
	require.NoError(t, ms.Flush())
}

func TestMaskedStreams_FinishPreservesRunError(t *testing.T) {
	useGlobalSecret(t, "masked-streams-secret-value")

	var stdout bytes.Buffer
	runErr := errors.New("process failed")

	ms := NewMaskedStreams(nil, &stdout, nil)
	_, err := ms.Stdout.Write([]byte("tail masked-streams-"))
	require.NoError(t, err)

	got := ms.Finish(runErr)
	assert.Same(t, runErr, got, "the run error is returned unchanged")
	assert.Equal(t, "tail masked-streams-", stdout.String(), "the held tail is still flushed on failure")

	ms = NewMaskedStreams(nil, &stdout, nil)
	assert.NoError(t, ms.Finish(nil))
}

type closedSinkWriter struct{}

func (closedSinkWriter) Write([]byte) (int, error) { return 0, errors.New("sink closed") }

func TestMaskedStreams_FinishReportsFlushError(t *testing.T) {
	useGlobalSecret(t, "masked-streams-secret-value")

	ms := NewMaskedStreams(nil, closedSinkWriter{}, nil)
	_, err := ms.Stdout.Write([]byte("masked-streams-"))
	require.NoError(t, err)

	assert.Error(t, ms.Finish(nil), "a flush failure surfaces when the run itself succeeded")
}

func TestMaskOptionsForStdin(t *testing.T) {
	assert.Empty(t, MaskOptionsForStdin(nil))
	assert.Empty(t, MaskOptionsForStdin(strings.NewReader("")))

	// A pipe is a file but not a terminal: the default line-boundary hold stays on.
	r, w, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
	assert.Empty(t, MaskOptionsForStdin(r))
}

func TestNewOutput_SecretSplitAcrossWritesIsMasked(t *testing.T) {
	const secret = "output-split-secret-value"
	useGlobalSecret(t, secret)

	var terminal, capture bytes.Buffer
	output := NewOutput(OutputOptions{
		Prefix: "node",
		Stdout: OutputSinks{Terminal: &terminal, Capture: &capture},
	})

	_, err := output.Stdout.Write([]byte("value=output-split-"))
	require.NoError(t, err)
	_, err = output.Stdout.Write([]byte("secret-value\n"))
	require.NoError(t, err)
	require.NoError(t, output.Flush())

	for name, got := range map[string]string{"terminal": terminal.String(), "capture": capture.String()} {
		assert.NotContains(t, got, secret, name)
		assert.Equal(t, "[node] value="+MaskReplacement+"\n", got, name)
	}
}

func TestNewOutput_FlushReleasesHeldTail(t *testing.T) {
	useGlobalSecret(t, "output-split-secret-value")

	var terminal bytes.Buffer
	output := NewOutput(OutputOptions{Stderr: OutputSinks{Terminal: &terminal}})

	_, err := output.Stderr.Write([]byte("output-split-"))
	require.NoError(t, err)
	assert.Empty(t, terminal.String())

	require.NoError(t, output.Flush())
	assert.Equal(t, "output-split-", terminal.String())
}

func TestStreamingMaskWriter_RecordsOutputForProcessStreams(t *testing.T) {
	const secret = "recorded-split-secret-value"
	useGlobalSecret(t, secret)

	f, err := os.CreateTemp(t.TempDir(), "stdout")
	require.NoError(t, err)
	oldStdout := os.Stdout
	os.Stdout = f
	t.Cleanup(func() { os.Stdout = oldStdout; _ = f.Close() })

	rec := &testRecorder{}
	restore := SetRecorder(rec)
	t.Cleanup(restore)

	sw := NewStreamingMaskWriter(os.Stdout)
	_, err = sw.Write([]byte("v=recorded-split-"))
	require.NoError(t, err)
	_, err = sw.Write([]byte("secret-value\n"))
	require.NoError(t, err)
	require.NoError(t, sw.Flush())

	assert.Equal(t, "o", rec.stream)
	assert.Equal(t, "v="+MaskReplacement+"\n", rec.data)
	assert.NotContains(t, rec.data, secret)
}
