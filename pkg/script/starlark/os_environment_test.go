package starlark

import (
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

func TestOSEnvironmentSnapshotAndIsolation(t *testing.T) {
	t.Parallel()
	const key = "ATMOS_OS_TEST_LOCAL_ENV"
	hostValue, hostPresent := os.LookupEnv(key)
	base := []string{key + "=old", "EMPTY=", "WITH_EQUALS=a=b", key + "=initial"}
	engine := New()
	result, err := engine.Execute(t.Context(), script.Spec{
		ProcessEnv: base, Env: map[string]string{key: "declared"},
		Source: `before = os.environ()
os.setenv("ATMOS_OS_TEST_LOCAL_ENV", "changed")
os.setenv("NEW", "")
after = os.environ()
output = [before["ATMOS_OS_TEST_LOCAL_ENV"], after["ATMOS_OS_TEST_LOCAL_ENV"], os.getenv("ATMOS_OS_TEST_LOCAL_ENV"), after["NEW"], after["EMPTY"], after["WITH_EQUALS"], env["ATMOS_OS_TEST_LOCAL_ENV"]]`,
	})
	require.NoError(t, err)
	assert.JSONEq(t, `["initial","changed","changed","","","a=b","declared"]`, result.Value)
	assert.Equal(t, []string{key + "=old", "EMPTY=", "WITH_EQUALS=a=b", key + "=initial"}, base)
	value, present := os.LookupEnv(key)
	assert.Equal(t, hostValue, value)
	assert.Equal(t, hostPresent, present)
	result, err = engine.Execute(t.Context(), script.Spec{ProcessEnv: base, Source: `output = os.getenv("ATMOS_OS_TEST_LOCAL_ENV")`})
	require.NoError(t, err)
	assert.Equal(t, "initial", result.Value)
	_, err = runSource(t, `snapshot = os.environ()
snapshot["NEW"] = "value"`)
	require.ErrorContains(t, err, "frozen")
}

func TestOSSetenvReachesProcessesAndSteps(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Times(6).DoAndReturn(func(_ context.Context, task process.TaskSpec) process.Result {
		assert.Contains(t, task.Env, "TOKEN="+task.Args[len(task.Args)-1])
		assert.NotContains(t, task.Env, "TOKEN=initial")
		assert.Contains(t, task.Env, "KEEP=kept")
		return process.Result{Started: true}
	})
	library := NewMockStepLibrary(gomock.NewController(t))
	library.EXPECT().Names().Return([]string{"example"})
	library.EXPECT().Fork().Return(library)
	library.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, call *api.StepCall) (*api.StepResult, error) {
		assert.Contains(t, call.ProcessEnv, "TOKEN=updated")
		return &api.StepResult{}, nil
	})
	result, err := New(WithProcessRunner(runner)).Execute(t.Context(), script.Spec{
		WorkingDirectory: dir, ProcessEnv: []string{"TOKEN=initial", "KEEP=kept"}, Steps: library,
		ResolveComponent: func(_ context.Context, ref script.ComponentRef) (*script.Component, error) {
			return &script.Component{ComponentRef: ref, Path: dir, Config: map[string]any{}, Env: map[string]string{"TOKEN": "component"}}, nil
		},
		Source: `component = components.get("api", "dev", "application")
os.setenv("TOKEN", "updated")
exec.run(["tool", "updated"])
atmos.run(["version", "updated"])
component.exec(["tool", "updated"])
exec.run(["tool", "call"], env={"TOKEN": "call"})
atmos.run(["version", "call"], env={"TOKEN": "call"})
component.exec(["tool", "call"], env={"TOKEN": "call"})
steps.example()
output = os.getenv("TOKEN")`,
	})
	require.NoError(t, err)
	assert.Equal(t, "updated", result.Value)
}

func TestOSEnvironIncludesPinnedTools(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	toolsDir := filepath.Join(dir, "tools")
	result, err := New().Execute(t.Context(), script.Spec{
		ProcessEnv: []string{"PATH=" + dir},
		InstallTools: func(context.Context, map[string]string) ([]string, error) {
			return []string{toolsDir}, nil
		},
		Source: `before = os.environ()
dependencies.tools("test/tool", "1")
output = [before["PATH"], os.environ()["PATH"], os.getenv("PATH")]`,
	})
	require.NoError(t, err)
	want := toolsDir + string(os.PathListSeparator) + dir
	assert.JSONEq(t, fmt.Sprintf(`[%q,%q,%q]`, dir, want, want), result.Value)
}
