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
	"syscall"
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

func TestBuildValidatesEveryTargetBeforeCompilation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, entries string
		options       []Option
		sentinel      error
	}{
		{name: "missing declaration", options: []Option{WithPlatform("linux/arm64")}, sentinel: errUtils.ErrPluginUnsupportedPlatform},
		{name: "empty matrix", options: []Option{WithAllPlatforms(true)}, sentinel: errUtils.ErrPluginInvalidConfig},
		{name: "conflicting selectors", options: []Option{WithPlatform("linux/arm64"), WithAllPlatforms(true)}, sentinel: errUtils.ErrPluginInvalidConfig},
		{name: "invalid variant", entries: "    linux/arm/v99: {executable: out/arm}\n", options: []Option{WithAllPlatforms(true)}, sentinel: errUtils.ErrPluginInvalidConfig},
		{name: "escaping executable", entries: "    linux/arm64: {executable: ../escape}\n", options: []Option{WithAllPlatforms(true)}, sentinel: errUtils.ErrPluginInvalidConfig},
		{name: "shared output", entries: "    linux/amd64: {executable: out/plugin}\n    linux/arm64: {executable: out/plugin}\n", options: []Option{WithAllPlatforms(true)}, sentinel: errUtils.ErrPluginInvalidConfig},
		{name: "case insensitive shared output", entries: "    linux/amd64: {executable: OUT/plugin}\n    linux/arm64: {executable: out/plugin}\n", options: []Option{WithAllPlatforms(true)}, sentinel: errUtils.ErrPluginInvalidConfig},
		{name: "first output is an ancestor", entries: "    linux/amd64: {executable: out/plugin}\n    linux/arm64: {executable: out/plugin/child}\n", options: []Option{WithAllPlatforms(true)}, sentinel: errUtils.ErrPluginInvalidConfig},
		{name: "second output is an ancestor", entries: "    linux/amd64: {executable: out/plugin/child}\n    linux/arm64: {executable: out/plugin}\n", options: []Option{WithAllPlatforms(true)}, sentinel: errUtils.ErrPluginInvalidConfig},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			directory := sourceFixture(t)
			if tc.entries != "" {
				raw := strings.Replace(buildManifest, "  compatibility:", "  platforms:\n"+tc.entries+"  compatibility:", 1)
				require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(raw), 0o600))
			}
			options := append([]Option{WithRunner(NewMockRunner(gomock.NewController(t)))}, tc.options...)
			outputs, err := CompileAll(t.Context(), directory, options...)
			require.ErrorIs(t, err, tc.sentinel)
			require.Empty(t, outputs)
			requireNoTemporaryOutputs(t, directory)
		})
	}
}

func TestDefaultBuildPinsCompilerTargetDespiteEnvironment(t *testing.T) {
	t.Setenv("GOOS", "invalid-host-os")
	t.Setenv("GOARCH", "invalid-host-arch")
	t.Setenv("GOAMD64", "v4")
	t.Setenv("GOARM64", "v9.5")
	directory := sourceFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "go.sum"), nil, 0o600))
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task process.TaskSpec) process.Result {
		require.Contains(t, task.Env, "GOOS="+runtime.GOOS)
		require.Contains(t, task.Env, "GOARCH="+runtime.GOARCH)
		require.Contains(t, task.Env, "CGO_ENABLED=0")
		require.NotContains(t, task.Env, "GOOS=invalid-host-os")
		require.NotContains(t, task.Env, "GOARCH=invalid-host-arch")
		require.NoError(t, os.WriteFile(task.Args[2], []byte("native build"), 0o755))
		return process.Result{Started: true}
	})
	outputs, err := CompileAll(t.Context(), directory, WithRunner(runner))
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Join(directory, "custom", "out", "plugin")}, outputs)
	raw, err := os.ReadFile(outputs[0])
	require.NoError(t, err)
	require.Equal(t, "native build", string(raw))
}

func TestBuildRejectsSymlinkOutputAliasesBeforeStartingCompiler(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, link, target, output string }{
		{name: "aliased directory", link: "alias", target: "real", output: "alias/plugin"},
		{name: "aliased executable", link: "alias", target: "real/plugin", output: "alias"},
		{name: "dangling executable", link: "alias", target: "real/missing", output: "alias"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			directory := sourceFixture(t)
			require.NoError(t, os.Mkdir(filepath.Join(directory, "real"), 0o755))
			previous := []byte("previous successful build")
			realOutput := filepath.Join(directory, "real", "plugin")
			require.NoError(t, os.WriteFile(realOutput, previous, 0o600))
			buildSymlink(t, filepath.Join(directory, tc.target), filepath.Join(directory, tc.link))
			entries := "  platforms:\n    linux/amd64: {executable: real/plugin}\n    linux/arm64: {executable: " + tc.output + "}\n"
			raw := strings.Replace(buildManifest, "  compatibility:", entries+"  compatibility:", 1)
			require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(raw), 0o600))
			runner := NewMockRunner(gomock.NewController(t))
			outputs, err := CompileAll(t.Context(), directory, WithRunner(runner), WithAllPlatforms(true))
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			require.Empty(t, outputs)
			require.Contains(t, strings.Join(errUtils.AllHints(err), " "), "symbolic link")
			current, err := os.ReadFile(realOutput)
			require.NoError(t, err)
			require.Equal(t, previous, current)
			requireNoTemporaryOutputs(t, directory)
		})
	}
}

func TestBuildAllowsSymlinkToSourceDirectory(t *testing.T) {
	t.Parallel()
	directory := sourceFixture(t)
	link := filepath.Join(t.TempDir(), "project")
	buildSymlink(t, directory, link)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "go.sum"), nil, 0o600))
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task process.TaskSpec) process.Result {
		require.NoError(t, os.WriteFile(task.Args[2], []byte("new build"), 0o755))
		return process.Result{Started: true}
	})
	executable, err := Compile(t.Context(), link, WithRunner(runner))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(link, "custom", "out", "plugin"), executable)
	contents, err := os.ReadFile(filepath.Join(directory, "custom", "out", "plugin"))
	require.NoError(t, err)
	require.Equal(t, "new build", string(contents))
}

func buildSymlink(t *testing.T, target, link string) {
	t.Helper()
	err := os.Symlink(target, link)
	// Windows ERROR_PRIVILEGE_NOT_HELD is not classified as os.ErrPermission.
	const privilegeNotHeld = syscall.Errno(1314)
	if runtime.GOOS == "windows" && (errors.Is(err, os.ErrPermission) || errors.Is(err, privilegeNotHeld)) {
		t.Skipf("creating symbolic links requires Windows privileges: %v", err)
	}
	require.NoError(t, err)
}
