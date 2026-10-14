package starlark

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestDependenciesToolsProcessEnvironment(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	toolDir := filepath.Join(dir, "tools")
	original := []string{"PATH=" + dir, "KEEP=value"}
	var installed atomic.Int32
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Times(4).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
		assert.Equal(t, int32(1), installed.Load())
		assert.Equal(t, toolDir+string(os.PathListSeparator)+dir, envpkg.SliceToMap(spec.Env)["PATH"])
		assert.Equal(t, "value", envpkg.SliceToMap(spec.Env)["KEEP"])
		return process.Result{}
	})
	_, err := New(WithProcessRunner(runner)).Execute(context.Background(), script.Spec{
		WorkingDirectory: dir, ProcessEnv: original,
		ResolveComponent: func(_ context.Context, ref script.ComponentRef) (*script.Component, error) {
			return &script.Component{ComponentRef: ref, Path: dir, Config: map[string]any{}}, nil
		},
		InstallTools: func(_ context.Context, tools map[string]string) ([]string, error) {
			assert.Equal(t, map[string]string{"jq": "1.7.1"}, tools)
			installed.Add(1)
			return []string{toolDir}, nil
		},
		Source: `component = components.get(name="api", stack="dev", type="application")
dependencies.tools("jq", "1.7.1")
dependencies.tools(name="jq", version="1.7.1")
component.exec(["jq", "--version"])
atmos.toolchain("install", "jq@1.7.1")
def branch():
    return exec.run(["jq", "--version"]).exit_code
output = steps.parallel(functions=[branch, branch], max_concurrency=2)
`,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"PATH=" + dir, "KEEP=value"}, original)
}

func TestDependenciesToolsErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source, want string
		installs           int
	}{
		{"empty name", `dependencies.tools("", "1")`, "nonempty", 0},
		{"empty version", `dependencies.tools("jq", " ")`, "nonempty", 0},
		{"missing version", `dependencies.tools("jq")`, "version", 0},
		{"conflict", "dependencies.tools(\"jq\", \"1\")\ndependencies.tools(\"jq\", \"2\")", "already pinned", 1},
		{"branch", "def branch():\n    dependencies.tools(\"jq\", \"1\")\nsteps.parallel(functions=[branch])", "before starting parallel", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			_, err := New().Execute(context.Background(), script.Spec{Source: tc.source, InstallTools: func(context.Context, map[string]string) ([]string, error) { calls.Add(1); return nil, nil }})
			require.ErrorContains(t, err, tc.want)
			assert.Equal(t, int32(tc.installs), calls.Load())
		})
	}
}

func TestDependenciesToolsFailureAndDryRun(t *testing.T) {
	t.Parallel()
	failure := errors.New("registry unavailable")
	for _, dryRun := range []bool{false, true} {
		var calls int
		_, err := New(WithProcessRunner(NewMockRunner(gomock.NewController(t)))).Execute(context.Background(), script.Spec{
			Source: "dependencies.tools(\"jq\", \"1.7.1\")\nexec.run([\"jq\"])", DryRun: dryRun,
			InstallTools: func(context.Context, map[string]string) ([]string, error) { calls++; return nil, failure },
		})
		if dryRun {
			require.NoError(t, err)
			assert.Zero(t, calls)
		} else {
			require.ErrorIs(t, err, failure)
			assert.Equal(t, 1, calls)
		}
	}
	_, err := runSource(t, `dependencies.tools("jq", "1.7.1")`)
	require.ErrorContains(t, err, "unavailable")
}

func TestDependenciesToolsLoadedModuleAndCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := New(WithReadFile(func(path string) ([]byte, error) {
		require.True(t, strings.HasSuffix(path, "tools.star"))
		return []byte("dependencies.tools(\"jq\", \"1.7.1\")\nready=True"), nil
	})).Execute(ctx, script.Spec{
		Source:       `load("tools.star", "ready")`,
		InstallTools: func(context.Context, map[string]string) ([]string, error) { cancel(); return []string{"unused"}, nil },
	})
	require.ErrorIs(t, err, context.Canceled)
}
