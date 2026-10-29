package starlark

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestOSChdirScopesFilesProcessesAndSteps(t *testing.T) {
	t.Parallel()
	hostDir, err := os.Getwd()
	require.NoError(t, err)
	initial := t.TempDir()
	child := filepath.Join(initial, "child")
	require.NoError(t, os.Mkdir(child, 0o700))
	componentDir := t.TempDir()
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Times(5).DoAndReturn(func(_ context.Context, task process.TaskSpec) process.Result {
		switch task.Args[len(task.Args)-1] {
		case "explicit":
			assert.Equal(t, initial, task.Dir)
		case "component":
			assert.Equal(t, componentDir, task.Dir)
		default:
			assert.Equal(t, child, task.Dir)
		}
		return process.Result{Started: true}
	})
	library := NewMockStepLibrary(gomock.NewController(t))
	library.EXPECT().Names().Return([]string{"example"})
	library.EXPECT().Fork().Return(library)
	library.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, call *api.StepCall) (*api.StepResult, error) {
		assert.Equal(t, child, call.WorkingDirectory)
		return &api.StepResult{}, nil
	})
	engine := New(WithProcessRunner(runner))
	result, err := engine.Execute(t.Context(), script.Spec{
		WorkingDirectory: initial, AtmosWorkingDirectory: componentDir, Steps: library,
		ResolveComponent: func(_ context.Context, ref script.ComponentRef) (*script.Component, error) {
			return &script.Component{ComponentRef: ref, Path: componentDir, Config: map[string]any{}}, nil
		},
		Source: `before = os.getcwd()
os.chdir("child")
fs.write_file("value.txt", "saved")
exec.run(["tool", "default"])
atmos.run(["version", "default"])
exec.run(["tool", "explicit"], working_directory="..")
atmos.run(["version", "explicit"], working_directory="..")
components.get("api", "dev", "application").exec(["tool", "component"])
steps.example()
output = [before, os.getcwd(), fs.resolve("."), fs.read_file("value.txt")]`,
	})
	require.NoError(t, err)
	assert.JSONEq(t, fmt.Sprintf(`[%q,%q,%q,"saved"]`, initial, child, child), result.Value)
	contents, err := os.ReadFile(filepath.Join(child, "value.txt"))
	require.NoError(t, err)
	assert.Equal(t, "saved", string(contents))
	currentHostDir, err := os.Getwd()
	require.NoError(t, err)
	assert.Equal(t, hostDir, currentHostDir)
	result, err = engine.Execute(t.Context(), script.Spec{WorkingDirectory: initial, Source: `output = os.getcwd()`})
	require.NoError(t, err)
	assert.Equal(t, initial, result.Value)
}

func TestOSChdirFailurePreservesDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file"), []byte("value"), 0o600))
	for _, path := range []string{"", "missing", "file", "bad\x00path"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			_, err := New().Execute(t.Context(), script.Spec{
				WorkingDirectory: dir, Stdout: &stdout,
				Source: fmt.Sprintf("defer(lambda: print(os.getcwd()))\nos.chdir(%q)", path),
			})
			require.ErrorContains(t, err, "os.chdir")
			assert.Equal(t, dir+"\n", stdout.String())
		})
	}
}

func TestOSStateInheritedByParallelTasks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(func(_ context.Context, task process.TaskSpec) process.Result {
		assert.Equal(t, dir, task.Dir)
		assert.Contains(t, task.Env, "INHERITED=parent")
		return process.Result{Started: true}
	})
	result, err := New(WithProcessRunner(runner)).Execute(t.Context(), script.Spec{
		Source: fmt.Sprintf(`os.chdir(%q)
os.setenv("INHERITED", "parent")
def child():
    exec.run(["tool"])
    return [os.getcwd(), os.getenv("INHERITED"), os.environ()["INHERITED"]]
output = steps.parallel(functions=[child, child])`, dir),
	})
	require.NoError(t, err)
	assert.JSONEq(t, fmt.Sprintf(`[[%q,"parent","parent"],[%q,"parent","parent"]]`, dir, dir), result.Value)
}

func TestOSMutationRejectedInParallel(t *testing.T) {
	t.Parallel()
	for _, call := range []string{`os.chdir(".")`, `os.setenv("TOKEN", "value")`} {
		t.Run(call, func(t *testing.T) {
			t.Parallel()
			_, err := runSource(t, "steps.parallel(functions=[lambda: "+call+"])")
			require.ErrorContains(t, err, "available only in the main thread")
		})
	}
}

func TestOSArgumentValidation(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`os.getcwd(1)`, `os.environ(1)`, `os.chdir(42)`, `os.setenv("KEY", 42)`,
		`os.setenv("", "value")`, `os.setenv("A=B", "value")`, `os.setenv("A\x00B", "value")`,
		`os.setenv("TOKEN", "private-payload\x00")`,
	} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			_, err := runSource(t, source)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "private-payload")
		})
	}
}

func TestOSChangesAffectWhich(t *testing.T) {
	t.Parallel()
	exe, err := os.Executable()
	require.NoError(t, err)
	dir, name := filepath.Dir(exe), filepath.Base(exe)
	result, err := New().Execute(t.Context(), script.Spec{
		WorkingDirectory: t.TempDir(), ProcessEnv: []string{"PATHEXT=" + os.Getenv("PATHEXT")},
		Source: fmt.Sprintf(`os.setenv("PATH", %q)
from_path = exec.which(%q)
os.chdir(%q)
output = [from_path, exec.which(%q)]`, dir, name, dir, filepath.Join("..", filepath.Base(dir), name)),
	})
	require.NoError(t, err)
	assert.JSONEq(t, fmt.Sprintf(`[%q,%q]`, exe, exe), result.Value)
}
