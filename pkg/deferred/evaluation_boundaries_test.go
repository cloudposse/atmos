package deferred

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/merge"
)

func TestFilterDeferredFieldsIncludesAncestorsAndDescendants(t *testing.T) {
	dctx := merge.NewDeferredMergeContext()
	dctx.AddDeferred([]string{"parent"}, "!ancestor")
	dctx.AddDeferred([]string{"parent", "child"}, "!selected")
	dctx.AddDeferred([]string{"parent", "child", "nested"}, "!descendant")
	dctx.AddDeferred([]string{"unused"}, "!unused")
	FilterDeferredFields(dctx, "vars", nil)
	require.Len(t, dctx.GetDeferredValues(), 4)
	FilterDeferredFields(dctx, "vars", [][]string{{"vars", "parent", "child"}})
	require.Len(t, dctx.GetDeferredValues(), 3)
	require.NotContains(t, dctx.GetDeferredValues(), "unused")
	FilterDeferredFields(dctx, "vars", [][]string{})
	require.Empty(t, dctx.GetDeferredValues())
}

func TestEvaluationDependenciesInListsAndDynamicScopes(t *testing.T) {
	data := map[string]any{
		"vars":     map[string]any{"values": []any{"{{ .settings.first }}", "plain", 1}},
		"settings": map[string]any{"first": "{{ .settings.second }}", "second": "{{ .settings.first }}"},
		"literal":  "plain",
	}
	paths := ExpandEvaluationPaths(data, [][]string{{"vars"}, {"vars"}, {"literal", "child"}})
	require.Contains(t, paths, []string{"settings", "first"})
	require.Contains(t, paths, []string{"settings", "second"})
	require.Len(t, paths, 5, "dependency traversal must stop on cycles and duplicate paths")
	require.Nil(t, ExpandEvaluationPaths(data, nil))
	for _, value := range []any{map[string]any{"dynamic": "{{ index . .key }}"}, []any{"{{ index . .key }}"}} {
		require.Nil(t, ExpandEvaluationPaths(map[string]any{"vars": value}, [][]string{{"vars"}}))
	}
	selected, excluded := SplitEvaluationFields(map[string]any{"vars": "!template produces map", "other": "literal"}, [][]string{{}, {"vars", "child"}})
	require.Equal(t, map[string]any{"vars": "!template produces map"}, selected)
	require.Equal(t, map[string]any{"other": "literal"}, excluded)
}

func TestSplitSectionsPreservesUnevaluatedValues(t *testing.T) {
	input := map[string]any{"vars": map[string]any{"id": "!remote"}, "metadata": map[string]any{"enabled": true}}
	selected, excluded := SplitSectionsByRequirement(input, nil)
	require.Equal(t, input, selected)
	require.Nil(t, excluded)
	selected, excluded = SplitSectionsByRequirement(input, []string{"metadata"})
	require.Equal(t, map[string]any{"metadata": input["metadata"]}, selected)
	require.Equal(t, map[string]any{"vars": input["vars"]}, excluded)
	RestoreEvaluationFields(selected, excluded)
	require.Equal(t, input, selected)
}
