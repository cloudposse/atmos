package hooks_test

import (
	"bytes"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	_ "github.com/cloudposse/atmos/pkg/workflow"
)

func TestTestHookPreservesGenericWith(t *testing.T) {
	for _, scope := range []string{"standalone", "direct", "parallel", "matrix"} {
		t.Run(scope, func(t *testing.T) {
			key := "{{ .env.TEST_KEY }}"
			keys := []string{"check"}
			if scope == "matrix" {
				key = "{{ .matrix.region }}"
				keys = []string{"east", "west"}
			}
			with := map[string]any{
				"store": "{{ .env.TEST_STORE }}", "key": key, "value": "{{ .env.TEST_VALUE }}",
				"stack": "test-stack", "component": "test-component",
			}
			children := []any{map[string]any{"name": "write", "type": "store", "with": with}}
			if scope == "parallel" || scope == "matrix" {
				group := map[string]any{"name": "group", "type": scope, "steps": children}
				if scope == "matrix" {
					group["matrix"] = map[string]any{"region": keys}
				}
				children = []any{group}
			}
			hook := &hooks.Hook{
				Kind: "step", Type: "test", OnFailure: "fail",
				Env:  map[string]string{"TEST_KEY": "check", "TEST_STORE": "results", "TEST_VALUE": "passed"},
				With: map[string]any{"name": "smoke", "steps": children},
			}
			if scope == "standalone" {
				hook.Type = "store"
				hook.With = with
			}
			backend := store.NewMockStore(gomock.NewController(t))
			for _, expectedKey := range keys {
				backend.EXPECT().Set("test-stack", "test-component", expectedKey, "passed").Return(nil)
			}
			kind, ok := hooks.GetKind("step")
			require.True(t, ok)
			var output bytes.Buffer
			_, err := kind.Engine.Run(&hooks.ExecContext{
				Hook: hook, Kind: kind, Event: hooks.AfterTerraformApply,
				AtmosConfig: &schema.AtmosConfiguration{Stores: store.StoreRegistry{"results": backend}},
				Info:        &schema.ConfigAndStacksInfo{}, Stdout: &output, Stderr: &output,
			})
			require.NoError(t, err)
			if scope != "standalone" {
				assert.Contains(t, ansi.Strip(output.String()), "0 failed")
			}
			assert.Equal(t, key, with["key"])
			assert.Equal(t, "{{ .env.TEST_VALUE }}", with["value"])
		})
	}
}
