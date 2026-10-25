package deferred

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
)

var errConcreteBoom = errors.New("boom")

// mergeConcrete exercises the real precedence rules rather than resolving isolated layers.
func mergeConcrete(t *testing.T, inputs []map[string]any, strategy string, processor m.YAMLFunctionProcessor, filter func(string) bool) (map[string]any, error) {
	t.Helper()
	config := &schema.AtmosConfiguration{Settings: schema.AtmosSettings{ListMergeStrategy: strategy}}
	result, dctx, err := m.MergeWithDeferred(config, inputs)
	require.NoError(t, err)
	err = m.ApplyDeferredMerges(dctx, result, config, WithConcreteResolution(processor, filter))
	return result, err
}

func TestConcreteResolution(t *testing.T) {
	filter := ConcreteStringFilter(nil, true)
	t.Run("renders surviving inherited values without mutating cached layers", func(t *testing.T) {
		inherited := map[string]any{
			"plain": "static", "stack": "{{ .atmos_stack }}", "repository": "!git.repository unknown",
			"nested": map[string]any{"stack": "{{ .atmos_stack }}"},
			"list":   []any{"{{ .atmos_stack }}", 3, true, nil},
		}
		config := &schema.AtmosConfiguration{}
		_, dctx, err := m.MergeWithDeferred(config, []map[string]any{{"tags": inherited}, {"tags": "!labels"}})
		require.NoError(t, err)
		processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
		processor.EXPECT().ProcessYAMLFunctionString("!labels").Return(map[string]any{"owner": "platform"}, nil).Times(2)
		processor.EXPECT().ProcessYAMLFunctionString("{{ .atmos_stack }}").Return("dev", nil).Times(6)
		processor.EXPECT().ProcessYAMLFunctionString("!git.repository unknown").Return("acme/infra", nil).Times(2)

		for range 2 {
			result := map[string]any{}
			require.NoError(t, m.ApplyDeferredMerges(dctx, result, config, WithConcreteResolution(processor, filter)))
			tags := result["tags"].(map[string]any)
			assert.Equal(t, map[string]any{
				"plain": "static", "stack": "dev", "repository": "acme/infra", "owner": "platform",
				"nested": map[string]any{"stack": "dev"}, "list": []any{"dev", 3, true, nil},
			}, tags)
			// Mutating a resolved result must not affect the cache or the next resolution.
			tags["nested"].(map[string]any)["stack"] = "changed"
		}
		assert.Equal(t, "{{ .atmos_stack }}", inherited["stack"])
		assert.Equal(t, "{{ .atmos_stack }}", inherited["nested"].(map[string]any)["stack"])
	})

	t.Run("does not evaluate replaced leaves or function results", func(t *testing.T) {
		processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
		processor.EXPECT().ProcessYAMLFunctionString("!labels").Return(map[string]any{
			"owner": "platform", "nested": map[string]any{"value": "ok"},
			"literal": "!exec do-not-run", "template": "{{ literal-output }}",
		}, nil)
		result, err := mergeConcrete(t, []map[string]any{
			{"tags": map[string]any{"owner": "{{ .vars.missing }}", "nested": map[string]any{"value": "!git.invalid"}}},
			{"tags": "!labels"},
		}, "", processor, filter)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{
			"owner": "platform", "nested": map[string]any{"value": "ok"},
			"literal": "!exec do-not-run", "template": "{{ literal-output }}",
		}, result["tags"])
	})

	for _, replacement := range []any{"", false, 0, nil, []any{}, map[string]any{"value": "ok"}} {
		t.Run("replacement retains its type", func(t *testing.T) {
			processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
			processor.EXPECT().ProcessYAMLFunctionString("!labels").Return(map[string]any{"owner": replacement}, nil)
			result, err := mergeConcrete(t, []map[string]any{
				{"tags": map[string]any{"owner": "{{ .vars.missing }}"}}, {"tags": "!labels"},
			}, "", processor, filter)
			require.NoError(t, err)
			assert.Equal(t, replacement, result["tags"].(map[string]any)["owner"])
		})
	}

	t.Run("higher concrete layers replace lower concrete templates", func(t *testing.T) {
		processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
		processor.EXPECT().ProcessYAMLFunctionString("!labels").Return(map[string]any{}, nil)
		processor.EXPECT().ProcessYAMLFunctionString("{{ .vars.owner }}").Return("platform", nil)
		result, err := mergeConcrete(t, []map[string]any{
			{"tags": map[string]any{"owner": "{{ .vars.missing }}"}}, {"tags": "!labels"},
			{"tags": map[string]any{"owner": "{{ .vars.owner }}"}},
		}, "", processor, filter)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"owner": "platform"}, result["tags"])
	})

	for _, strategy := range []string{"", m.ListMergeStrategyReplace, m.ListMergeStrategyMerge, m.ListMergeStrategyAppend} {
		t.Run("list strategy "+strategy, func(t *testing.T) {
			processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
			processor.EXPECT().ProcessYAMLFunctionString("!template list").Return([]any{map[string]any{"owner": "new"}}, nil)
			want := []any{map[string]any{"owner": "new"}}
			if strategy == m.ListMergeStrategyAppend {
				processor.EXPECT().ProcessYAMLFunctionString("{{ .vars.old }}").Return("old", nil)
				want = []any{map[string]any{"owner": "old"}, map[string]any{"owner": "new"}}
			}
			result, err := mergeConcrete(t, []map[string]any{
				{"items": []any{map[string]any{"owner": "{{ .vars.old }}"}}}, {"items": "!template list"},
			}, strategy, processor, filter)
			require.NoError(t, err)
			assert.Equal(t, want, result["items"])
		})
	}

	t.Run("templates disabled still resolves inherited functions", func(t *testing.T) {
		processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
		processor.EXPECT().ProcessYAMLFunctionString("!labels").Return(map[string]any{}, nil)
		processor.EXPECT().ProcessYAMLFunctionString("!git.repository unknown").Return("acme/infra", nil)
		result, err := mergeConcrete(t, []map[string]any{
			{"tags": map[string]any{"stack": "{{ .atmos_stack }}", "repository": "!git.repository unknown"}}, {"tags": "!labels"},
		}, "", processor, ConcreteStringFilter(nil, false))
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"stack": "{{ .atmos_stack }}", "repository": "acme/infra"}, result["tags"])
	})

	for _, inherited := range []any{
		map[string]any{"broken": "{{ missing }}"},
		map[string]any{"nested": map[string]any{"broken": "{{ missing }}"}},
		map[string]any{"list": []any{"{{ missing }}"}},
		map[string]any{"list": []any{map[string]any{"broken": "{{ missing }}"}}},
	} {
		t.Run("surviving errors propagate", func(t *testing.T) {
			processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
			processor.EXPECT().ProcessYAMLFunctionString("!labels").Return(map[string]any{}, nil)
			processor.EXPECT().ProcessYAMLFunctionString("{{ missing }}").Return(nil, errConcreteBoom)
			_, err := mergeConcrete(t, []map[string]any{{"tags": inherited}, {"tags": "!labels"}}, "", processor, filter)
			require.ErrorIs(t, err, errConcreteBoom)
			assert.Contains(t, err.Error(), "inherited values at tags")
		})
	}

	t.Run("scalar layers are replaced without rendering", func(t *testing.T) {
		processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
		processor.EXPECT().ProcessYAMLFunctionString("!labels").Return(map[string]any{"owner": "platform"}, nil)
		result, err := mergeConcrete(t, []map[string]any{{"tags": "{{ missing }}"}, {"tags": "!labels"}}, "", processor, filter)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"owner": "platform"}, result["tags"])
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
		{"template enabled", "x-{{ .vars.stage }}", true, true},
		{"template disabled", "x-{{ .vars.stage }}", false, false},
		{"plain", "static", true, false},
		{"empty", "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ConcreteStringFilter(nil, tc.templates)(tc.value))
		})
	}
	custom := ConcreteStringFilter([]string{"${", "}"}, true)
	assert.True(t, custom("a ${ b }"))
	assert.False(t, custom("a {{ b }}"))
}

func TestWithConcreteResolution_NilArguments(t *testing.T) {
	assert.Nil(t, WithConcreteResolution(nil, ConcreteStringFilter(nil, true)))
	processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
	assert.Same(t, processor, WithConcreteResolution(processor, nil))
}
