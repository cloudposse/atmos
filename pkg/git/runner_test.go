package git

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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

	result, err := NewExecRunner().Run(context.Background(), "git", []string{"version"}, &RunOptions{})
	require.NoError(t, err)
	assert.Contains(t, result.Stdout, "git version")
	assert.Equal(t, 0, result.ExitCode)
}

func TestExecRunnerNonZeroExitWrapsSentinel(t *testing.T) {
	requireGit(t)

	// `git rev-parse` outside a repository exits non-zero.
	dir := t.TempDir()
	var stderr bytes.Buffer
	result, err := NewExecRunner().Run(context.Background(), "git", []string{"rev-parse", "--verify", "HEAD"}, &RunOptions{Dir: dir, Stderr: &stderr})

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitCommandExited))
	assert.NotEqual(t, 0, result.ExitCode)
	// Stderr is streamed to the writer and mirrored into the bounded tail,
	// but never embedded in the error message.
	assert.Equal(t, stderr.String(), result.StderrTail)
	assert.NotContains(t, err.Error(), strings.TrimSpace(stderr.String()))
}

func TestExecRunnerMissingBinaryWrapsSentinel(t *testing.T) {
	_, err := NewExecRunner().Run(context.Background(), "definitely-not-a-real-binary-atmos", nil, &RunOptions{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitCommandFailed))
}

func TestExecRunnerDoesNotDiscoverParentGitRepo(t *testing.T) {
	requireGit(t)

	root := t.TempDir()
	initCmd := exec.Command("git", "init")
	initCmd.Dir = root
	out, err := initCmd.CombinedOutput()
	require.NoError(t, err, string(out))

	child := filepath.Join(root, "cache", "repositories", "deploy")
	require.NoError(t, os.MkdirAll(child, 0o755))

	result, err := NewExecRunner().Run(context.Background(), "git", []string{"status", "--porcelain"}, &RunOptions{Dir: child})

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitCommandExited))
	assert.Contains(t, result.StderrTail, "not a git repository")
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

// helperExecutor routes every command to the test binary so no platform binary is needed.
type helperExecutor struct{}

func (helperExecutor) LookPath(file string) (string, error) { return file, nil }

func (helperExecutor) CommandContext(ctx context.Context, _ string, _ ...string) *exec.Cmd {
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	return exec.CommandContext(ctx, exe)
}

// failingWriter simulates a diagnostic sink that rejects writes.
type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestExecRunnerFlushesUnterminatedMetadataLine(t *testing.T) {
	sinkErr := errors.New("sink rejected write")
	env := func(exit string) []string {
		return append(os.Environ(), helperStderrEnv+"=remote: no trailing newline", helperExitEnv+"="+exit)
	}

	t.Run("flush failure on a successful command is reported", func(t *testing.T) {
		result, err := NewExecRunnerWithExecutor(helperExecutor{}).Run(t.Context(), "git", nil, &RunOptions{Env: env("0"), CaptureHTTPMetadata: true, Stderr: failingWriter{err: sinkErr}})
		require.ErrorIs(t, err, sinkErr)
		require.ErrorIs(t, err, errUtils.ErrGitCommandFailed)
		require.NotNil(t, result.HTTP)
	})

	t.Run("flush failure does not mask a non-zero exit", func(t *testing.T) {
		result, err := NewExecRunnerWithExecutor(helperExecutor{}).Run(t.Context(), "git", nil, &RunOptions{Env: env("3"), CaptureHTTPMetadata: true, Stderr: failingWriter{err: sinkErr}})
		require.ErrorIs(t, err, errUtils.ErrGitCommandExited)
		require.NotErrorIs(t, err, sinkErr)
		require.Equal(t, 3, result.ExitCode)
	})

	t.Run("unterminated line is forwarded to a healthy sink", func(t *testing.T) {
		var output bytes.Buffer
		_, err := NewExecRunnerWithExecutor(helperExecutor{}).Run(t.Context(), "git", nil, &RunOptions{Env: env("0"), CaptureHTTPMetadata: true, Stderr: &output})
		require.NoError(t, err)
		require.Equal(t, "remote: no trailing newline\n", output.String())
	})
}
