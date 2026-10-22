package build

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/process"
)

//go:generate go run go.uber.org/mock/mockgen -destination mock_process_test.go -package build github.com/cloudposse/atmos/pkg/process Runner

func TestBuildUsesManifestPathAndStreamsCompilerOutput(t *testing.T) {
	t.Parallel()
	directory := sourceFixture(t)
	runner := NewMockRunner(gomock.NewController(t))
	var output bytes.Buffer
	expectModulePreparation(t, runner, directory, "")
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task process.TaskSpec) process.Result {
		require.Equal(t, "go", task.Command)
		require.Equal(t, directory, task.Dir)
		require.Len(t, task.Args, 4)
		require.Equal(t, []string{"build", "-o"}, task.Args[:2])
		require.Equal(t, ".", task.Args[3])
		require.Equal(t, directory, filepath.Dir(task.Args[2]))
		_, err := io.WriteString(task.Streams.Stderr, "compiler diagnostic")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(task.Args[2], []byte("compiled plugin"), 0o755))
		return process.Result{Started: true}
	})
	executable, err := Compile(t.Context(), directory, WithRunner(runner), WithStreams(process.Streams{Stderr: &output}))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(directory, "custom", "out", "plugin"), executable)
	compiled, err := os.ReadFile(filepath.Join(directory, "custom", "out", "plugin"))
	require.NoError(t, err)
	require.Equal(t, "compiled plugin", string(compiled))
	require.Equal(t, "compiler diagnostic", output.String())
	requireNoTemporaryOutputs(t, directory)
}

func TestBuildFailurePreservesExistingExecutable(t *testing.T) {
	t.Parallel()
	for _, result := range []process.Result{
		{Err: exec.ErrNotFound, ExitCode: -1},
		{Err: errors.New("compiler rejected source"), ExitCode: 2, Started: true},
	} {
		t.Run(result.Err.Error(), func(t *testing.T) {
			t.Parallel()
			directory := sourceFixture(t)
			destination := filepath.Join(directory, "custom", "out", "plugin")
			require.NoError(t, os.MkdirAll(filepath.Dir(destination), 0o755))
			require.NoError(t, os.WriteFile(destination, []byte("previous build"), 0o755))
			runner := NewMockRunner(gomock.NewController(t))
			expectModulePreparation(t, runner, directory, "")
			runner.EXPECT().Run(gomock.Any(), gomock.Any()).Return(result)
			executable, err := Compile(t.Context(), directory, WithRunner(runner))
			require.ErrorIs(t, err, errUtils.ErrPluginBuild)
			require.Empty(t, executable, "failed builds must never report a successful output path")
			require.ErrorIs(t, err, result.Err)
			if result.ExitCode > 0 {
				require.Equal(t, result.ExitCode, errUtils.GetExitCode(err))
			}
			previous, err := os.ReadFile(destination)
			require.NoError(t, err)
			require.Equal(t, "previous build", string(previous))
			requireNoTemporaryOutputs(t, directory)
		})
	}
}

func TestBuildCancellationDoesNotPublishOutput(t *testing.T) {
	t.Parallel()
	directory := sourceFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	runner := NewMockRunner(gomock.NewController(t))
	expectModulePreparation(t, runner, directory, "")
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, process.TaskSpec) process.Result {
		cancel()
		return process.Result{Started: true}
	})
	require.ErrorIs(t, Run(ctx, directory, WithRunner(runner)), context.Canceled)
	_, err := os.Stat(filepath.Join(directory, "custom", "out", "plugin"))
	require.ErrorIs(t, err, os.ErrNotExist)
	requireNoTemporaryOutputs(t, directory)
	// A previously canceled request must not start another compiler process.
	require.ErrorIs(t, Run(ctx, directory, WithRunner(runner)), context.Canceled)
}

func TestBuildRejectsInvalidSourceBeforeStartingCompiler(t *testing.T) {
	t.Parallel()
	t.Run("missing manifest", func(t *testing.T) {
		t.Parallel()
		require.ErrorIs(t, Run(t.Context(), t.TempDir(), WithRunner(NewMockRunner(gomock.NewController(t)))), errUtils.ErrPluginInvalidConfig)
	})
	t.Run("missing module", func(t *testing.T) {
		t.Parallel()
		directory := sourceFixture(t)
		require.NoError(t, os.Remove(filepath.Join(directory, "go.mod")))
		require.ErrorIs(t, Run(t.Context(), directory, WithRunner(NewMockRunner(gomock.NewController(t)))), errUtils.ErrPluginBuild)
	})
	t.Run("nil runner", func(t *testing.T) {
		t.Parallel()
		require.ErrorIs(t, Run(t.Context(), sourceFixture(t), WithRunner(nil)), errUtils.ErrPluginInvalidConfig)
	})
	t.Run("invalid extension", func(t *testing.T) {
		t.Parallel()
		directory := sourceFixture(t)
		raw := bytes.ReplaceAll([]byte(buildManifest), []byte("commands:"), []byte("unsupported:"))
		require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), raw, 0o600))
		require.ErrorIs(t, Run(t.Context(), directory, WithRunner(NewMockRunner(gomock.NewController(t)))), errUtils.ErrPluginUnsupportedExtension)
	})
}

func TestBuildRejectsOutputDirectoryObstructionBeforeStartingCompiler(t *testing.T) {
	t.Parallel()
	directory := sourceFixture(t)
	obstruction := filepath.Join(directory, "custom")
	require.NoError(t, os.WriteFile(obstruction, []byte("preserve user data"), 0o600))
	executable, err := Compile(t.Context(), directory, WithRunner(NewMockRunner(gomock.NewController(t))))
	require.Error(t, err)
	require.Empty(t, executable)
	data, err := os.ReadFile(obstruction)
	require.NoError(t, err)
	require.Equal(t, "preserve user data", string(data))
	requireNoTemporaryOutputs(t, directory)
}

func sourceFixture(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(buildManifest), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "go.mod"), []byte("module example.com/plugin\n"), 0o600))
	return directory
}

func requireNoTemporaryOutputs(t *testing.T, directory string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(directory, ".atmos-plugin-build-*"))
	require.NoError(t, err)
	require.Empty(t, matches)
}

const buildManifest = `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata: {name: example/build, version: 1.0.0}
spec:
  runtime: {type: native, protocol: 1, executable: custom/out/plugin}
  compatibility: {protocol: ">=1 <2"}
  capabilities:
    commands:
      about: {description: Build fixture}
`

func TestBuildUsesCurrentPlatformExecutable(t *testing.T) {
	t.Parallel()
	directory := sourceFixture(t)
	manifest := strings.Replace(buildManifest, "  compatibility:", "  platforms:\n    "+runtime.GOOS+"/"+runtime.GOARCH+": {executable: platform/plugin}\n  compatibility:", 1)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(manifest), 0o600))
	runner := NewMockRunner(gomock.NewController(t))
	expectModulePreparation(t, runner, directory, "")
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task process.TaskSpec) process.Result {
		require.NoError(t, os.WriteFile(task.Args[2], []byte("platform build"), 0o755))
		return process.Result{Started: true}
	})
	executable, err := Compile(t.Context(), directory, WithRunner(runner))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(directory, "platform", "plugin"), executable)
	contents, err := os.ReadFile(executable)
	require.NoError(t, err)
	require.Equal(t, "platform build", string(contents))
	_, err = os.Stat(filepath.Join(directory, "custom", "out", "plugin"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func expectModulePreparation(t *testing.T, runner *MockRunner, directory, workspace string) {
	t.Helper()
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task process.TaskSpec) process.Result {
		require.Equal(t, "go", task.Command)
		require.Equal(t, directory, task.Dir)
		require.Equal(t, []string{"env", "GOWORK"}, task.Args)
		_, err := io.WriteString(task.Streams.Stdout, workspace)
		require.NoError(t, err)
		return process.Result{Started: true}
	})
	if workspace != "" && workspace != "off" {
		return
	}
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task process.TaskSpec) process.Result {
		require.Equal(t, "go", task.Command)
		require.Equal(t, directory, task.Dir)
		require.Equal(t, []string{"mod", "tidy"}, task.Args)
		return process.Result{Started: true}
	})
}

func TestBuildWorkspaceSkipsStandaloneModulePreparation(t *testing.T) {
	t.Parallel()
	directory := sourceFixture(t)
	runner := NewMockRunner(gomock.NewController(t))
	expectModulePreparation(t, runner, directory, filepath.Join(t.TempDir(), "go.work"))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task process.TaskSpec) process.Result {
		require.Equal(t, []string{"build", "-o"}, task.Args[:2], "workspace builds must not use the unsupported -mod flag")
		require.NoError(t, os.WriteFile(task.Args[2], []byte("workspace plugin"), 0o755))
		return process.Result{Started: true}
	})
	executable, err := Compile(t.Context(), directory, WithRunner(runner))
	require.NoError(t, err)
	raw, err := os.ReadFile(executable)
	require.NoError(t, err)
	require.Equal(t, "workspace plugin", string(raw))
}

func TestBuildModulePreparationFailurePreservesExecutable(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"env", "tidy"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			directory := sourceFixture(t)
			destination := filepath.Join(directory, "custom", "out", "plugin")
			require.NoError(t, os.MkdirAll(filepath.Dir(destination), 0o755))
			require.NoError(t, os.WriteFile(destination, []byte("last successful build"), 0o755))
			runner := NewMockRunner(gomock.NewController(t))
			if stage == "tidy" {
				runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task process.TaskSpec) process.Result {
					require.Equal(t, []string{"env", "GOWORK"}, task.Args)
					_, err := io.WriteString(task.Streams.Stdout, "off\n")
					require.NoError(t, err)
					return process.Result{Started: true}
				})
			}
			failure := errors.New("module preparation failed")
			runner.EXPECT().Run(gomock.Any(), gomock.Any()).Return(process.Result{Started: true, ExitCode: 1, Err: failure})
			executable, err := Compile(t.Context(), directory, WithRunner(runner))
			require.ErrorIs(t, err, failure)
			require.ErrorIs(t, err, errUtils.ErrPluginBuild)
			require.Empty(t, executable)
			raw, err := os.ReadFile(destination)
			require.NoError(t, err)
			require.Equal(t, "last successful build", string(raw))
			requireNoTemporaryOutputs(t, directory)
		})
	}
}

func TestBuildPreparesStandaloneTransitiveModuleGraph(t *testing.T) {
	t.Setenv("GOWORK", "off")
	directory := sourceFixture(t)
	// The generated scaffold declares direct dependencies only. A standalone build
	// must discover indirect requirements without a preexisting go.sum or workspace.
	require.NoError(t, os.WriteFile(filepath.Join(directory, "go.mod"), []byte(`module example.com/plugin

go 1.26.0
require example.com/direct v1.0.0
replace example.com/direct => ./direct
replace example.com/indirect => ./indirect
`), 0o600))
	for name, source := range map[string]string{
		"main.go":              "package main\nimport _ \"example.com/direct\"\nfunc main() {}\n",
		"direct/go.mod":        "module example.com/direct\ngo 1.26.0\nrequire example.com/indirect v1.0.0\n",
		"direct/direct.go":     "package direct\nimport _ \"example.com/indirect\"\n",
		"indirect/go.mod":      "module example.com/indirect\ngo 1.26.0\n",
		"indirect/indirect.go": "package indirect\n",
	} {
		target := filepath.Join(directory, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
		require.NoError(t, os.WriteFile(target, []byte(source), 0o600))
	}
	var diagnostics bytes.Buffer
	executable, err := Compile(t.Context(), directory, WithStreams(process.Streams{Stderr: &diagnostics}))
	require.NoError(t, err, diagnostics.String())
	require.FileExists(t, executable)
	module, err := os.ReadFile(filepath.Join(directory, "go.mod"))
	require.NoError(t, err)
	require.Contains(t, string(module), "example.com/indirect v1.0.0 // indirect")
}

func TestBuildPreservesExistingModuleMetadata(t *testing.T) {
	t.Parallel()
	directory := sourceFixture(t)
	original, err := os.ReadFile(filepath.Join(directory, "go.mod"))
	require.NoError(t, err)
	checksum := []byte("existing module checksums\n")
	require.NoError(t, os.WriteFile(filepath.Join(directory, "go.sum"), checksum, 0o600))
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task process.TaskSpec) process.Result {
		require.Equal(t, []string{"build", "-o"}, task.Args[:2], "existing modules must not run tidy")
		require.NoError(t, os.WriteFile(task.Args[2], []byte("compiled plugin"), 0o755))
		return process.Result{Started: true}
	})
	require.NoError(t, Run(t.Context(), directory, WithRunner(runner)))
	module, err := os.ReadFile(filepath.Join(directory, "go.mod"))
	require.NoError(t, err)
	require.Equal(t, original, module)
	sums, err := os.ReadFile(filepath.Join(directory, "go.sum"))
	require.NoError(t, err)
	require.Equal(t, checksum, sums)
}
