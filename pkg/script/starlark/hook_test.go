package starlark

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestHookComponentExecUsesSnapshot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
		assert.Equal(t, dir, spec.Dir)
		assert.Equal(t, "smoke-test", spec.Command)
		assert.Equal(t, "hook", envpkg.SliceToMap(spec.Env)["SHARED"])
		assert.Equal(t, "call", envpkg.SliceToMap(spec.Env)["OVERRIDE"])
		return process.Result{}
	})
	_, err := New(WithProcessRunner(runner)).Execute(context.Background(), script.Spec{
		Hook: &script.HookContext{
			Name: "smoke", Event: "after.terraform.apply",
			ProcessOverrides: map[string]string{"SHARED": "hook"},
			Component: &script.Component{
				ComponentRef: script.ComponentRef{Name: "api", Stack: "dev", Type: "terraform"},
				Path:         dir, Config: map[string]any{}, Env: map[string]string{"SHARED": "component"},
			},
		},
		Env:    map[string]string{"OVERRIDE": "step"},
		Source: `ctx.component.exec(["smoke-test"], env={"OVERRIDE": "call"})`,
	})
	require.NoError(t, err)
}

func TestNonHookContextIsExplicitlyAbsent(t *testing.T) {
	t.Parallel()
	result, err := runSource(t, `output = [ctx.hook, ctx.operation, ctx.component]`)
	require.NoError(t, err)
	assert.JSONEq(t, `[null,null,null]`, result.Value)
}

func TestHookContextSnapshotIsolation(t *testing.T) {
	t.Parallel()
	settings := map[string]any{"enabled": true}
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, process.TaskSpec) process.Result {
		// The script has already constructed its immutable snapshot.
		settings["enabled"] = false
		return process.Result{}
	})
	result, err := New(WithProcessRunner(runner)).Execute(context.Background(), script.Spec{
		Hook: &script.HookContext{Component: &script.Component{Config: map[string]any{"settings": settings}}},
		Source: `exec.run(["mutate-source"])
output = ctx.component.settings["enabled"]`,
	})
	require.NoError(t, err)
	assert.Equal(t, "true", result.Value)
	assert.Equal(t, false, settings["enabled"])
}
