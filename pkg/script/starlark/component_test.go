package starlark

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestParallelComponentFunctions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
		assert.Equal(t, "deploy", spec.Command)
		assert.Equal(t, filepath.Join(dir, "shared-implementation"), spec.Dir)
		assert.Contains(t, spec.Env, "COMPONENT="+spec.Args[0])
		assert.Equal(t, "step", envpkg.SliceToMap(spec.Env)["EXPLICIT"])
		assert.Equal(t, "override", envpkg.SliceToMap(spec.Env)["COMMAND"])
		return process.Result{}
	})
	result, err := New(WithProcessRunner(runner)).Execute(context.Background(), script.Spec{
		Component: &script.ComponentRef{Name: "api", Stack: "dev", Type: "application"},
		Env:       map[string]string{"EXPLICIT": "step"}, ProcessEnv: []string{"EXPLICIT=parent"},
		ProcessOverrides: map[string]string{"COMMAND": "override"},
		ResolveComponent: func(_ context.Context, ref script.ComponentRef) (*script.Component, error) {
			assert.Equal(t, "application", ref.Type)
			assert.Equal(t, "dev", ref.Stack)
			return &script.Component{
				ComponentRef: ref, Implementation: "shared-implementation", Path: filepath.Join(dir, "shared-implementation"),
				Config: map[string]any{"vars": map[string]any{"name": ref.Name}}, Env: map[string]string{"COMPONENT": ref.Name, "EXPLICIT": "component", "COMMAND": "component"},
			}, nil
		},
		Source: `
def deploy(name):
    component = components.get(name=name, stack=ctx.component.stack, type="application")
    component.exec(["deploy", component.vars["name"]])
    return {"name": component.name, "implementation": component.implementation}
output = steps.parallel(tasks=[steps.task(name=n, function=deploy, args=[n]) for n in ["api", "worker"]])`,
	})
	require.NoError(t, err)
	assert.JSONEq(t, `[{"name":"api","implementation":"shared-implementation"},{"name":"worker","implementation":"shared-implementation"}]`, result.Value)
}

func TestComponentConfigIsReadOnly(t *testing.T) {
	t.Parallel()
	_, err := New().Execute(context.Background(), script.Spec{
		Component: &script.ComponentRef{Name: "api", Stack: "dev", Type: "application"},
		ResolveComponent: func(_ context.Context, ref script.ComponentRef) (*script.Component, error) {
			return &script.Component{ComponentRef: ref, Config: map[string]any{"vars": map[string]any{"name": "api"}}}, nil
		},
		Source: `ctx.component.vars["name"] = "changed"`,
	})
	require.ErrorContains(t, err, "frozen")
}

func TestComponentLookupValidation(t *testing.T) {
	t.Parallel()
	for source, want := range map[string]error{
		`components.get(name="",stack="dev",type="application")`:    errUtils.ErrStarlarkInvalidArgument,
		`components.get(name="api",stack="dev",type="application")`: errUtils.ErrStarlark,
	} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			_, err := runSource(t, source)
			require.ErrorIs(t, err, want)
		})
	}
	for _, config := range []map[string]any{nil, {"unsupported": func() {}}} {
		t.Run(fmt.Sprint(config == nil), func(t *testing.T) {
			t.Parallel()
			_, err := New().Execute(context.Background(), script.Spec{
				Source:    `ctx.component.name`,
				Component: &script.ComponentRef{Name: "api", Stack: "dev", Type: "application"},
				ResolveComponent: func(_ context.Context, ref script.ComponentRef) (*script.Component, error) {
					return &script.Component{ComponentRef: ref, Config: config}, nil
				},
			})
			require.ErrorIs(t, err, errUtils.ErrStarlark)
		})
	}
}
