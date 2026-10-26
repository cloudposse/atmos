package deferred

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/function/yamlsource"
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

func TestConcreteResolution_SurvivingValues(t *testing.T) {
	filter := ConcreteStringFilter(nil, true)
	labels := (yamlsource.Source{Tag: "!labels"}).Encode()
	repository := (yamlsource.Source{Tag: "!git.repository", Argument: "unknown"}).Encode()
	inherited := map[string]any{
		"plain": "static", "stack": "{{ .atmos_stack }}", "repository": repository,
		"nested": map[string]any{"stack": "{{ .atmos_stack }}"},
		"list":   []any{"{{ .atmos_stack }}", 3, true, nil},
	}
	config := &schema.AtmosConfiguration{}
	_, dctx, err := m.MergeWithDeferred(config, []map[string]any{{"tags": inherited}, {"tags": labels}})
	require.NoError(t, err)
	processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
	processor.EXPECT().ProcessYAMLFunctionString(labels).Return(map[string]any{"owner": "platform"}, nil).Times(2)
	processor.EXPECT().ProcessYAMLFunctionString("{{ .atmos_stack }}").Return("dev", nil).Times(6)
	processor.EXPECT().ProcessYAMLFunctionString(repository).Return("acme/infra", nil).Times(2)

	for range 2 {
		result := map[string]any{}
		require.NoError(t, m.ApplyDeferredMerges(dctx, result, config, WithConcreteResolution(processor, filter)))
		require.IsType(t, map[string]any{}, result["tags"])
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
}

func TestConcreteResolution_ReplacedValues(t *testing.T) {
	filter := ConcreteStringFilter(nil, true)
	labels := (yamlsource.Source{Tag: "!labels"}).Encode()
	processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
	processor.EXPECT().ProcessYAMLFunctionString(labels).Return(map[string]any{
		"owner": "platform", "nested": map[string]any{"value": "ok"},
		"literal": "!exec do-not-run", "template": "{{ literal-output }}",
	}, nil)
	result, err := mergeConcrete(t, []map[string]any{
		{"tags": map[string]any{"owner": "{{ .vars.missing }}", "nested": map[string]any{"value": "!git.invalid"}}},
		{"tags": labels},
	}, "", processor, filter)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"owner": "platform", "nested": map[string]any{"value": "ok"},
		"literal": "!exec do-not-run", "template": "{{ literal-output }}",
	}, result["tags"])
}

func TestConcreteResolution_ReplacementTypes(t *testing.T) {
	filter := ConcreteStringFilter(nil, true)
	labels := (yamlsource.Source{Tag: "!labels"}).Encode()
	for _, replacement := range []any{"", false, 0, nil, []any{}, map[string]any{"value": "ok"}} {
		t.Run("replacement retains its type", func(t *testing.T) {
			processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
			processor.EXPECT().ProcessYAMLFunctionString(labels).Return(map[string]any{"owner": replacement}, nil)
			result, err := mergeConcrete(t, []map[string]any{
				{"tags": map[string]any{"owner": "{{ .vars.missing }}"}}, {"tags": labels},
			}, "", processor, filter)
			require.NoError(t, err)
			assert.Equal(t, replacement, result["tags"].(map[string]any)["owner"])
		})
	}
}

func TestConcreteResolution_ConcretePrecedence(t *testing.T) {
	filter := ConcreteStringFilter(nil, true)
	labels := (yamlsource.Source{Tag: "!labels"}).Encode()
	processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
	processor.EXPECT().ProcessYAMLFunctionString(labels).Return(map[string]any{}, nil)
	processor.EXPECT().ProcessYAMLFunctionString("{{ .vars.owner }}").Return("platform", nil)
	result, err := mergeConcrete(t, []map[string]any{
		{"tags": map[string]any{"owner": "{{ .vars.missing }}"}},
		{"tags": labels},
		{"tags": map[string]any{"owner": "{{ .vars.owner }}"}},
	}, "", processor, filter)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"owner": "platform"}, result["tags"])
}

func TestConcreteResolution_ListStrategies(t *testing.T) {
	filter := ConcreteStringFilter(nil, true)
	listTemplate := (yamlsource.Source{Tag: "!template", Argument: "list"}).Encode()
	for _, strategy := range []string{"", m.ListMergeStrategyReplace, m.ListMergeStrategyMerge, m.ListMergeStrategyAppend} {
		t.Run("list strategy "+strategy, func(t *testing.T) {
			processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
			processor.EXPECT().ProcessYAMLFunctionString(listTemplate).Return([]any{map[string]any{"owner": "new"}}, nil)
			want := []any{map[string]any{"owner": "new"}}
			if strategy == m.ListMergeStrategyAppend {
				processor.EXPECT().ProcessYAMLFunctionString("{{ .vars.old }}").Return("old", nil)
				want = []any{map[string]any{"owner": "old"}, map[string]any{"owner": "new"}}
			}
			result, err := mergeConcrete(t, []map[string]any{
				{"items": []any{map[string]any{"owner": "{{ .vars.old }}"}}}, {"items": listTemplate},
			}, strategy, processor, filter)
			require.NoError(t, err)
			assert.Equal(t, want, result["items"])
		})
	}
}

func TestConcreteResolution_TemplatesDisabled(t *testing.T) {
	labels := (yamlsource.Source{Tag: "!labels"}).Encode()
	repository := (yamlsource.Source{Tag: "!git.repository", Argument: "unknown"}).Encode()
	processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
	processor.EXPECT().ProcessYAMLFunctionString(labels).Return(map[string]any{}, nil)
	processor.EXPECT().ProcessYAMLFunctionString(repository).Return("acme/infra", nil)
	result, err := mergeConcrete(t, []map[string]any{
		{"tags": map[string]any{"stack": "{{ .atmos_stack }}", "repository": repository}}, {"tags": labels},
	}, "", processor, ConcreteStringFilter(nil, false))
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"stack": "{{ .atmos_stack }}", "repository": "acme/infra"}, result["tags"])
}

func TestConcreteResolution_SurvivingErrors(t *testing.T) {
	filter := ConcreteStringFilter(nil, true)
	labels := (yamlsource.Source{Tag: "!labels"}).Encode()
	for _, inherited := range []any{
		map[string]any{"broken": "{{ missing }}"},
		map[string]any{"nested": map[string]any{"broken": "{{ missing }}"}},
		map[string]any{"list": []any{"{{ missing }}"}},
		map[string]any{"list": []any{map[string]any{"broken": "{{ missing }}"}}},
	} {
		t.Run("surviving errors propagate", func(t *testing.T) {
			processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
			processor.EXPECT().ProcessYAMLFunctionString(labels).Return(map[string]any{}, nil)
			processor.EXPECT().ProcessYAMLFunctionString("{{ missing }}").Return(nil, errConcreteBoom)
			_, err := mergeConcrete(t, []map[string]any{{"tags": inherited}, {"tags": labels}}, "", processor, filter)
			require.ErrorIs(t, err, errConcreteBoom)
			assert.Contains(t, err.Error(), "inherited values at tags")
		})
	}
}

func TestConcreteResolution_ReplacedScalar(t *testing.T) {
	filter := ConcreteStringFilter(nil, true)
	labels := (yamlsource.Source{Tag: "!labels"}).Encode()
	processor := NewMockYAMLFunctionProcessor(gomock.NewController(t))
	processor.EXPECT().ProcessYAMLFunctionString(labels).Return(map[string]any{"owner": "platform"}, nil)
	result, err := mergeConcrete(t, []map[string]any{{"tags": "{{ missing }}"}, {"tags": labels}}, "", processor, filter)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"owner": "platform"}, result["tags"])
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
