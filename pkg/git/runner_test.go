package git

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
}

func TestExecRunnerCapturesStdout(t *testing.T) {
	requireGit(t)

	result, err := NewExecRunner().Run(context.Background(), "git", []string{"version"}, RunOptions{})
	require.NoError(t, err)
	assert.Contains(t, result.Stdout, "git version")
	assert.Equal(t, 0, result.ExitCode)
}

func TestExecRunnerNonZeroExitWrapsSentinel(t *testing.T) {
	requireGit(t)

	// `git rev-parse` outside a repository exits non-zero.
	dir := t.TempDir()
	var stderr bytes.Buffer
	result, err := NewExecRunner().Run(context.Background(), "git", []string{"rev-parse", "--verify", "HEAD"}, RunOptions{Dir: dir, Stderr: &stderr})

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitCommandExited))
	assert.NotEqual(t, 0, result.ExitCode)
	// Stderr is streamed to the writer and mirrored into the bounded tail,
	// but never embedded in the error message.
	assert.Equal(t, stderr.String(), result.StderrTail)
	assert.NotContains(t, err.Error(), strings.TrimSpace(stderr.String()))
}

func TestExecRunnerMissingBinaryWrapsSentinel(t *testing.T) {
	_, err := NewExecRunner().Run(context.Background(), "definitely-not-a-real-binary-atmos", nil, RunOptions{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitCommandFailed))
}

func TestTailBufferKeepsTail(t *testing.T) {
	tail := &tailBuffer{limit: 8}

	_, err := tail.Write([]byte("0123456789abcdef"))
	require.NoError(t, err)
	assert.Equal(t, "89abcdef", tail.String())

	_, err = tail.Write([]byte("ZZ"))
	require.NoError(t, err)
	assert.Equal(t, "abcdefZZ", tail.String())
}

func TestFirstArgSkipsFlags(t *testing.T) {
	assert.Equal(t, "push", firstArg([]string{"push", "origin", "main"}))
	assert.Equal(t, "clone", firstArg([]string{"--depth", "clone"})) // flags skipped.
	assert.Equal(t, "", firstArg(nil))
}
