package shell

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	metricsprocess "github.com/cloudposse/atmos/pkg/metrics/process"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecuteCommandOutputRouting(t *testing.T) {
	for _, tc := range []struct {
		name            string
		redirect        string
		wantOut         string
		wantErr         string
		wantMetadataErr string
	}{
		{name: "separate streams", wantOut: "stdout", wantErr: "stderr", wantMetadataErr: "stderr"},
		{name: "explicit stderr", redirect: "/dev/stderr", wantOut: "stdout", wantErr: "stderr", wantMetadataErr: "stderr"},
		{name: "merged streams", redirect: "/dev/stdout", wantOut: "both", wantMetadataErr: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var terminalOut, terminalErr, override, captureOut, captureErr, metadataOut, metadataErr bytes.Buffer
			helper := shellHelperCommand(t, "stdout-stderr")
			err := ExecuteCommand(&CommandRequest{
				AtmosConfig: schema.AtmosConfiguration{Diagnostics: schema.Diagnostics{Enabled: true, IncludeOutput: true, File: filepath.Join(t.TempDir(), "events.jsonl")}},
				Command:     helper.command, Args: helper.args, Env: helper.env, RedirectStderr: tc.redirect,
			}, WithProcessStreams(process.Streams{Stdout: &terminalOut, Stderr: &terminalErr}), WithStdoutOverride(&override), WithStdoutCapture(&captureOut), WithStderrCapture(&captureErr), WithExecMetadataOutputCapture(&metadataOut, &metadataErr))
			require.NoError(t, err)
			assert.Empty(t, terminalOut.String(), "override must replace the terminal stdout sink")
			assert.Equal(t, tc.wantErr, terminalErr.String())
			assert.Equal(t, "stderr", captureErr.String())
			assert.Equal(t, tc.wantMetadataErr, metadataErr.String())
			if tc.wantOut == "both" {
				assert.Contains(t, override.String(), "stdout")
				assert.Contains(t, override.String(), "stderr")
				assert.Len(t, override.String(), len("stdoutstderr"))
			} else {
				assert.Equal(t, tc.wantOut, override.String())
			}
			assert.Equal(t, override.String(), captureOut.String())
			assert.Equal(t, override.String(), metadataOut.String(), "merged stderr must be captured exactly once")
		})
	}
}

func TestExecuteCommandFailureAndDryRun(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(*testing.T, *CommandRequest) []ShellCommandOption
		check   func(*testing.T, error)
	}{
		{name: "dry run does not resolve missing executable", prepare: func(_ *testing.T, r *CommandRequest) []ShellCommandOption {
			r.Command = "atmos-command-that-does-not-exist"
			r.DryRun = true
			return nil
		}, check: func(t *testing.T, err error) { require.NoError(t, err) }},
		{name: "missing executable preserves path error", prepare: func(t *testing.T, r *CommandRequest) []ShellCommandOption {
			r.Command = filepath.Join(t.TempDir(), "absent")
			return nil
		}, check: func(t *testing.T, err error) { require.ErrorIs(t, err, os.ErrNotExist) }},
		{name: "stderr open failure", prepare: func(t *testing.T, r *CommandRequest) []ShellCommandOption {
			r.RedirectStderr = filepath.Join(t.TempDir(), "absent", "stderr")
			return nil
		}, check: func(t *testing.T, err error) { require.ErrorIs(t, err, os.ErrNotExist) }},
		{name: "canceled context", prepare: func(_ *testing.T, _ *CommandRequest) []ShellCommandOption {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return []ShellCommandOption{WithProcessContext(ctx)}
		}, check: func(t *testing.T, err error) { require.ErrorIs(t, err, context.Canceled) }},
		{name: "exit code survives wrapper", prepare: func(t *testing.T, r *CommandRequest) []ShellCommandOption {
			helper := shellHelperCommand(t, "exit")
			r.Args = helper.args
			return nil
		}, check: func(t *testing.T, err error) {
			var exit errUtils.ExitCodeError
			require.ErrorAs(t, err, &exit)
			assert.Equal(t, 2, exit.Code)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helper := shellHelperCommand(t, "stdout-stderr")
			req := CommandRequest{Command: helper.command, Args: helper.args, Env: helper.env}
			opts := []ShellCommandOption{WithProcessStreams(process.Streams{Stdout: io.Discard, Stderr: io.Discard})}
			opts = append(opts, tc.prepare(t, &req)...)
			tc.check(t, ExecuteCommand(&req, opts...))
		})
	}
}

func TestExecuteCommandSanitizedEnvironmentAndMetrics(t *testing.T) {
	helper := shellHelperCommand(t, "stdout-stderr")
	var stdout bytes.Buffer
	var metrics *metricsprocess.ProcessMetrics
	var calls int
	err := ExecuteCommand(
		&CommandRequest{Command: helper.command, Args: helper.args},
		WithEnvironment(helper.env),
		WithProcessStreams(process.Streams{Stdout: &stdout, Stderr: io.Discard}),
		WithMetricsCallback(func(value *metricsprocess.ProcessMetrics) { calls++; metrics = value }),
	)
	require.NoError(t, err)
	assert.Equal(t, "stdout", stdout.String(), "the supplied environment must reach the helper process")
	assert.Equal(t, 1, calls)
	require.NotNil(t, metrics)
	assert.Positive(t, metrics.WallTime)
}

func TestExecuteCommandSnapshotsRequestBeforeOptions(t *testing.T) {
	helper := shellHelperCommand(t, "stdout-stderr")
	request := CommandRequest{Command: helper.command, Args: helper.args, Env: helper.env}
	var stdout bytes.Buffer
	err := ExecuteCommand(
		&request,
		WithProcessStreams(process.Streams{Stdout: &stdout, Stderr: io.Discard}),
		func(*CommandConfig) { request.Command = "missing-command-after-snapshot" },
	)
	require.NoError(t, err)
	assert.Equal(t, "stdout", stdout.String())
	assert.Equal(t, "missing-command-after-snapshot", request.Command)
}

func TestClosePacedWritersClosesAllAndKeepsFirstError(t *testing.T) {
	first, err := os.CreateTemp(t.TempDir(), "closed")
	require.NoError(t, err)
	require.NoError(t, first.Close())
	last, err := os.CreateTemp(t.TempDir(), "open")
	require.NoError(t, err)
	defer last.Close()
	err = closePacedWriters([]io.Closer{first, last})
	require.ErrorIs(t, err, os.ErrClosed)
	var pathErr *os.PathError
	require.ErrorAs(t, err, &pathErr)
	assert.Equal(t, first.Name(), pathErr.Path)
	_, writeErr := last.Write([]byte("after close"))
	require.ErrorIs(t, writeErr, os.ErrClosed, "later closers must still run after the first failure")
}

func TestLegacyCommandMatchesTypedRequest(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
		dryRun  bool
	}{
		{name: "output and environment", command: "stdout-stderr"},
		{name: "exit status", command: "exit"},
		{name: "dry run", command: "stdout-stderr", dryRun: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helper := shellHelperCommand(t, tc.command)
			var legacyOut, legacyErr, typedOut, typedErr bytes.Buffer
			legacyFailure := ExecuteShellCommand(schema.AtmosConfiguration{}, helper.command, helper.args, "", helper.env, tc.dryRun, "", WithProcessStreams(process.Streams{Stdout: &legacyOut, Stderr: &legacyErr}))
			typedFailure := ExecuteCommand(&CommandRequest{Command: helper.command, Args: helper.args, Env: helper.env, DryRun: tc.dryRun}, WithProcessStreams(process.Streams{Stdout: &typedOut, Stderr: &typedErr}))
			assert.Equal(t, typedOut.String(), legacyOut.String())
			assert.Equal(t, typedErr.String(), legacyErr.String())
			if typedFailure == nil {
				require.NoError(t, legacyFailure)
			} else {
				require.ErrorIs(t, legacyFailure, typedFailure)
			}
		})
	}
}
