package deferred

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
)

// errConcreteBoom is the failure a processor returns in the error-path test.
var errConcreteBoom = errors.New("boom")

// newLabelsOverrideContext builds a deferred context for a map that a higher-precedence layer
// overrides with `!labels`, so the lower layer is kept as a concrete layer.
func newLabelsOverrideContext(t *testing.T, inherited map[string]any) *m.DeferredMergeContext {
	t.Helper()
	_, dctx, err := m.MergeWithDeferred(&schema.AtmosConfiguration{}, []map[string]any{
		{"tags": inherited},
		{"tags": "!labels"},
	})
	require.NoError(t, err)
	return dctx
}

// concreteLayer returns the lower-precedence, non-function layer at the `tags` path.
func concreteLayer(t *testing.T, dctx *m.DeferredMergeContext) *m.DeferredValue {
	t.Helper()
	for _, layer := range dctx.GetDeferredValues()["tags"] {
		if !layer.IsFunction {
			return layer
		}
	}
	require.Fail(t, "no concrete layer found at the tags path")
	return nil
}

// renderingProcessor resolves the strings the tests care about and leaves the rest untouched.
func renderingProcessor(t *testing.T) *MockYAMLFunctionProcessor {
	t.Helper()
	processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
	processor.EXPECT().ProcessYAMLFunctionString(gomock.Any()).DoAndReturn(func(s string) (any, error) {
		switch s {
		case "{{ .atmos_stack }}":
			return "plat-ue2-dev", nil
		case "!git.repository unknown":
			return "acme/infra", nil
		default:
			return s, nil
		}
	}).AnyTimes()
	return processor
}

func TestResolveConcreteLayers(t *testing.T) {
	filter := ConcreteStringFilter(nil, true)

	t.Run("resolves templates and functions inside the inherited map", func(t *testing.T) {
		inherited := map[string]any{
			"plain":      "static",
			"stack":      "{{ .atmos_stack }}",
			"repository": "!git.repository unknown",
			"nested":     map[string]any{"stack": "{{ .atmos_stack }}"},
			"list":       []any{"{{ .atmos_stack }}", "last"},
		}
		dctx := newLabelsOverrideContext(t, inherited)

		require.NoError(t, ResolveConcreteLayers(dctx, renderingProcessor(t), filter))

		got, ok := concreteLayer(t, dctx).Value.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "static", got["plain"])
		assert.Equal(t, "plat-ue2-dev", got["stack"])
		assert.Equal(t, "acme/infra", got["repository"])
		assert.Equal(t, map[string]any{"stack": "plat-ue2-dev"}, got["nested"])
		assert.Equal(t, []any{"plat-ue2-dev", "last"}, got["list"])
	})

	t.Run("does not mutate the caller's maps in either direction", func(t *testing.T) {
		inherited := map[string]any{"stack": "{{ .atmos_stack }}", "nested": map[string]any{"stack": "{{ .atmos_stack }}"}}
		dctx := newLabelsOverrideContext(t, inherited)

		require.NoError(t, ResolveConcreteLayers(dctx, renderingProcessor(t), filter))

		// Result to source: resolving left the original input untouched.
		assert.Equal(t, "{{ .atmos_stack }}", inherited["stack"])
		assert.Equal(t, "{{ .atmos_stack }}", inherited["nested"].(map[string]any)["stack"])

		// Source to result: mutating the original after resolving does not change the resolved layer.
		inherited["stack"] = "mutated"
		inherited["nested"].(map[string]any)["stack"] = "mutated"
		got := concreteLayer(t, dctx).Value.(map[string]any)
		assert.Equal(t, "plat-ue2-dev", got["stack"])
		assert.Equal(t, "plat-ue2-dev", got["nested"].(map[string]any)["stack"])
	})

	t.Run("leaves function layers and scalar layers alone", func(t *testing.T) {
		_, dctx, err := m.MergeWithDeferred(&schema.AtmosConfiguration{}, []map[string]any{
			{"name": "{{ .atmos_stack }}"},
			{"name": "!labels"},
		})
		require.NoError(t, err)
		processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
		processor.EXPECT().ProcessYAMLFunctionString(gomock.Any()).Times(0)

		require.NoError(t, ResolveConcreteLayers(dctx, processor, filter))
	})

	t.Run("without template context only functions are resolved", func(t *testing.T) {
		dctx := newLabelsOverrideContext(t, map[string]any{
			"stack":      "{{ .atmos_stack }}",
			"repository": "!git.repository unknown",
		})

		require.NoError(t, ResolveConcreteLayers(dctx, renderingProcessor(t), ConcreteStringFilter(nil, false)))

		got := concreteLayer(t, dctx).Value.(map[string]any)
		assert.Equal(t, "{{ .atmos_stack }}", got["stack"], "templating is unavailable, so the template stays as written")
		assert.Equal(t, "acme/infra", got["repository"])
	})

	t.Run("custom delimiters", func(t *testing.T) {
		custom := ConcreteStringFilter([]string{"${", "}"}, true)
		assert.True(t, custom("a ${ b }"))
		assert.False(t, custom("a {{ b }}"))
	})

	t.Run("processor error is returned", func(t *testing.T) {
		dctx := newLabelsOverrideContext(t, map[string]any{"stack": "{{ .atmos_stack }}"})
		processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
		processor.EXPECT().ProcessYAMLFunctionString(gomock.Any()).Return(nil, errConcreteBoom)

		err := ResolveConcreteLayers(dctx, processor, filter)
		require.ErrorIs(t, err, errConcreteBoom)
	})

	t.Run("nil arguments are a no-op", func(t *testing.T) {
		require.NoError(t, ResolveConcreteLayers(nil, renderingProcessor(t), filter))
		require.NoError(t, ResolveConcreteLayers(newLabelsOverrideContext(t, map[string]any{"a": "{{ x }}"}), nil, filter))
		require.NoError(t, ResolveConcreteLayers(newLabelsOverrideContext(t, map[string]any{"a": "{{ x }}"}), renderingProcessor(t), nil))
	})
}

func TestConcreteStringFilter(t *testing.T) {
	for _, tc := range []struct {
		name      string
		value     string
		templates bool
		want      bool
	}{
		{"yaml function", "!git.sha", false, true},
		{"template with templating on", "x-{{ .vars.stage }}", true, true},
		{"template with templating off", "x-{{ .vars.stage }}", false, false},
		{"plain string", "static", true, false},
		{"empty string", "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ConcreteStringFilter(nil, tc.templates)(tc.value), strings.TrimSpace(tc.value))
		})
	}
}
