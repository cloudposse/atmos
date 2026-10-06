package starlark

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/script"
)

func TestComponentHandlesPrintCompactly(t *testing.T) {
	t.Parallel()
	resolver := func(_ context.Context, ref script.ComponentRef) (*script.Component, error) {
		return &script.Component{
			ComponentRef: ref, Path: "/components/api",
			Config: map[string]any{"vars": map[string]any{"name": ref.Name}, "atmos_cli_config": map[string]any{"secret": "do-not-print"}},
		}, nil
	}
	hookComponent := &script.Component{
		ComponentRef: script.ComponentRef{Name: "api", Stack: "dev", Type: "app"},
		Config:       map[string]any{"vars": map[string]any{"name": "api"}, "atmos_cli_config": map[string]any{"secret": "do-not-print"}},
	}
	tests := []struct {
		name string
		spec script.Spec
		expr string
	}{
		{"hook handle", script.Spec{Hook: &script.HookContext{Name: "h", Event: "after.terraform.plan", Component: hookComponent}}, "ctx.component"},
		{"lazy handle", script.Spec{Component: &script.ComponentRef{Name: "api", Stack: "dev", Type: "app"}}, "ctx.component"},
		{"components.get handle", script.Spec{}, `components.get(name = "api", stack = "dev", type = "app")`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.spec.Name, tt.spec.ResolveComponent = "compact.star", resolver
			tt.spec.Source = "output = [str(" + tt.expr + "), " + tt.expr + ".vars[\"name\"], " + tt.expr + ".path != None, type(" + tt.expr + ")]"
			result, err := New().Execute(t.Context(), tt.spec)
			require.NoError(t, err)
			assert.NotContains(t, result.Value, "do-not-print")
			assert.JSONEq(t, `["component(name = \"api\", stack = \"dev\", type = \"app\")","api",true,"struct"]`, result.Value)
		})
	}
}

func TestResolvedComponentHandleKeepsAllAttributes(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	result, err := executeComponent(t, `output = {"names": sorted(dir(components.get(name = "api", stack = "dev", type = "app"))), "str": str(ctx.component)}`, &calls, true)
	require.NoError(t, err)
	assert.JSONEq(t, `{"names":["config","env","exec","implementation","metadata","name","path","settings","stack","type","vars"],"str":"`+
		`component(name = \"api\", stack = \"dev\", type = \"app\")"}`, result.Value)
}
