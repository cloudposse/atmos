package build

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/process"
)

func parallelFixture(t *testing.T) string {
	t.Helper()
	directory := sourceFixture(t)
	entries := "  platforms:\n"
	for _, platform := range []string{"darwin/amd64", "darwin/arm64", "linux/amd64", "linux/arm64", "windows/amd64"} {
		entries += "    " + platform + ": {executable: bin/" + strings.ReplaceAll(platform, "/", "-") + "}\n"
	}
	raw := strings.Replace(buildManifest, "  compatibility:", entries+"  compatibility:", 1)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(raw), 0o600))
	return directory
}

func TestCompileAllRunsBoundedParallelBuilds(t *testing.T) {
	directory := parallelFixture(t)
	runner := NewMockRunner(gomock.NewController(t))
	expectModulePreparation(t, runner, directory, "")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var started, active atomic.Int32
	ready := make(chan struct{})
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, task process.TaskSpec) process.Result {
		assert.LessOrEqual(t, active.Add(1), int32(maxConcurrentBuilds))
		defer active.Add(-1)
		if started.Add(1) == maxConcurrentBuilds {
			close(ready)
		}
		select {
		case <-ready:
		case <-ctx.Done():
			return process.Result{Err: ctx.Err()}
		}
		var target string
		for _, value := range task.Env {
			if strings.HasPrefix(value, "GOOS=") || strings.HasPrefix(value, "GOARCH=") {
				target += value + "\n"
			}
		}
		_, outErr := io.WriteString(task.Streams.Stdout, "built\n")
		_, errErr := io.WriteString(task.Streams.Stderr, "checked\n")
		assert.NoError(t, outErr)
		assert.NoError(t, errErr)
		return process.Result{Started: true, Err: os.WriteFile(task.Args[2], []byte(target), 0o755)}
	}).Times(5)
	var stdout, stderr bytes.Buffer
	paths, err := CompileAll(ctx, directory, WithRunner(runner), WithAllPlatforms(true), WithStreams(process.Streams{Stdout: &stdout, Stderr: &stderr}))
	require.NoError(t, err)
	require.Len(t, paths, 5)
	require.IsIncreasing(t, paths)
	require.Equal(t, strings.Repeat("built\n", 5), stdout.String())
	require.Equal(t, strings.Repeat("checked\n", 5), stderr.String())
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		parts := strings.Split(filepath.Base(path), "-")
		require.Contains(t, string(raw), "GOOS="+parts[0]+"\n")
		require.Contains(t, string(raw), "GOARCH="+parts[1]+"\n")
	}
	requireNoTemporaryOutputs(t, directory)
}

func TestCompileAllPreservesFailedTargetAndReportsSuccessfulTargets(t *testing.T) {
	directory := parallelFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "go.sum"), nil, 0o600))
	previous := filepath.Join(directory, "bin", "windows-amd64")
	require.NoError(t, os.MkdirAll(filepath.Dir(previous), 0o755))
	require.NoError(t, os.WriteFile(previous, []byte("previous"), 0o755))
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task process.TaskSpec) process.Result {
		for _, value := range task.Env {
			if value == "GOOS=windows" {
				return process.Result{Started: true, ExitCode: 1, Err: errUtils.ErrProcessWaitFailed}
			}
		}
		return process.Result{Started: true, Err: os.WriteFile(task.Args[2], []byte("new"), 0o755)}
	}).Times(5)
	paths, err := CompileAll(t.Context(), directory, WithRunner(runner), WithAllPlatforms(true))
	require.ErrorIs(t, err, errUtils.ErrPluginBuild)
	require.Len(t, paths, 4)
	raw, err := os.ReadFile(previous)
	require.NoError(t, err)
	require.Equal(t, "previous", string(raw))
	requireNoTemporaryOutputs(t, directory)
}

func TestCompileAllStopsWhenDependencyPreparationFails(t *testing.T) {
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Return(process.Result{Err: errUtils.ErrProcessStartFailed})
	paths, err := CompileAll(t.Context(), parallelFixture(t), WithRunner(runner), WithAllPlatforms(true))
	require.ErrorIs(t, err, errUtils.ErrPluginBuild)
	require.Empty(t, paths)
}

func TestCompileAllCancellationAfterPreparationStartsNoCompilers(t *testing.T) {
	directory := parallelFixture(t)
	runner := NewMockRunner(gomock.NewController(t))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task process.TaskSpec) process.Result {
		require.Equal(t, []string{"env", "GOWORK"}, task.Args)
		_, err := io.WriteString(task.Streams.Stdout, filepath.Join(directory, "go.work"))
		require.NoError(t, err)
		cancel()
		return process.Result{Started: true}
	})
	paths, err := CompileAll(ctx, directory, WithRunner(runner), WithAllPlatforms(true))
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, paths)
	require.NoDirExists(t, filepath.Join(directory, "bin"))
	requireNoTemporaryOutputs(t, directory)
}

func TestCompileAllReportsDiagnosticWriteFailuresWithoutLosingBuiltPaths(t *testing.T) {
	directory := parallelFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "go.sum"), nil, 0o600))
	output, err := os.Create(filepath.Join(t.TempDir(), "diagnostics"))
	require.NoError(t, err)
	require.NoError(t, output.Close())
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task process.TaskSpec) process.Result {
		_, err := io.WriteString(task.Streams.Stdout, "compiler output\n")
		assert.NoError(t, err)
		return process.Result{Started: true, Err: os.WriteFile(task.Args[2], []byte("compiled"), 0o755)}
	}).Times(5)
	paths, err := CompileAll(t.Context(), directory, WithRunner(runner), WithAllPlatforms(true), WithStreams(process.Streams{Stdout: output}))
	require.ErrorIs(t, err, os.ErrClosed)
	expected := []string{"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64", "windows-amd64"}
	for index := range expected {
		expected[index] = filepath.Join(directory, "bin", expected[index])
	}
	require.Equal(t, expected, paths)
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "compiled", string(raw))
	}
	requireNoTemporaryOutputs(t, directory)
}
