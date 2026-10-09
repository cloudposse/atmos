package deferred

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/merge"
)

// TestWildcardEvaluationSelectsDeclarationFields retains unknown declaration names
// while excluding unrelated fields and discovering supported template dependencies.
func TestWildcardEvaluationSelectsDeclarationFields(t *testing.T) {
	paths := [][]string{{"secrets", "vars", "*", "store"}, {"secrets", "providers"}}
	first := map[string]any{"store": "{{ .vars.backend }}", "value": "{{ index . .dynamic }}"}
	second := map[string]any{"store": "other", "value": "!remote ignored"}
	input := map[string]any{
		"secrets": map[string]any{"vars": map[string]any{"FIRST": first, "SECOND": second}, "providers": map[string]any{"local": map[string]any{"spec": "{{ .vars.options }}"}}},
		"vars":    map[string]any{"backend": "{{ .vars.name }}", "name": "vault", "options": "kept", "unused": "!remote untouched"},
	}
	expanded := ExpandEvaluationPaths(input, paths)
	require.NotNil(t, expanded, "an excluded dynamic expression must not expand evaluation to the whole component")
	require.Contains(t, expanded, []string{"vars", "backend"})
	require.Contains(t, expanded, []string{"vars", "name"})
	require.Contains(t, expanded, []string{"vars", "options"})
	selected, excluded := SplitEvaluationFields(input, expanded)
	vars := selected["secrets"].(map[string]any)["vars"].(map[string]any)
	assert.Equal(t, map[string]any{"store": "{{ .vars.backend }}"}, vars["FIRST"])
	assert.Equal(t, map[string]any{"store": "other"}, vars["SECOND"])
	assert.NotContains(t, selected["vars"], "unused")
	assert.Contains(t, selected["secrets"], "providers")
	RestoreEvaluationFields(selected, excluded)
	assert.Equal(t, input, selected, "unrequested raw fields must remain available in the returned config")
}

// TestWildcardDeferredFields preserves ancestor producers and selected descendants,
// including declarations only materialized after a deferred function runs.
func TestWildcardDeferredFields(t *testing.T) {
	ctx := merge.NewDeferredMergeContext()
	ctx.AddDeferred([]string{"vars"}, "!template ancestor")
	ctx.AddDeferred([]string{"vars", "DYNAMIC", "store"}, "!remote selected")
	ctx.AddDeferred([]string{"vars", "DYNAMIC", "store", "nested"}, "!remote descendant")
	ctx.AddDeferred([]string{"vars", "DYNAMIC", "value"}, "!remote ignored")
	FilterDeferredFields(ctx, "secrets", [][]string{{"secrets", "vars", "*", "store"}})
	values := ctx.GetDeferredValues()
	require.Len(t, values, 3)
	assert.NotContains(t, values, "vars.DYNAMIC.value")
	generated := map[string]any{"secrets": map[string]any{"vars": map[string]any{"DYNAMIC": map[string]any{"store": "!selected", "value": "!ignored"}}}}
	selected, excluded := SplitEvaluationFields(generated, [][]string{{"secrets", "vars", "*", "store"}})
	assert.Equal(t, map[string]any{"store": "!selected"}, selected["secrets"].(map[string]any)["vars"].(map[string]any)["DYNAMIC"])
	RestoreEvaluationFields(selected, excluded)
	assert.Equal(t, generated, selected)
}

// TestWildcardAncestorDependencies discovers inputs needed to materialize a map,
// handles empty or absent maps, and retains conservative dynamic-scope evaluation.
func TestWildcardAncestorDependencies(t *testing.T) {
	paths := [][]string{{"secrets", "vars", "*", "store"}}
	for _, tc := range []struct {
		name       string
		value      any
		dependency bool
		dynamic    bool
	}{
		{name: "ancestor template", value: "{{ .vars.declarations }}", dependency: true},
		{name: "empty map", value: map[string]any{}},
		{name: "absent map"},
		{name: "selected dynamic template", value: map[string]any{"KEY": map[string]any{"store": "{{ index . .key }}"}}, dynamic: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := map[string]any{"secrets": map[string]any{"vars": tc.value}, "vars": map[string]any{"declarations": "data"}}
			expanded := ExpandEvaluationPaths(input, paths)
			if tc.dynamic {
				assert.Nil(t, expanded)
				return
			}
			require.NotNil(t, expanded)
			if tc.dependency {
				assert.Contains(t, expanded, []string{"vars", "declarations"})
			}
		})
	}
}
