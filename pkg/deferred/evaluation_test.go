package deferred

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestEvaluationDemand(t *testing.T) {
	input := map[string]any{
		"vars":     map[string]any{"name": "{{ .settings.owner }}", "unused": "!aws.account_id"},
		"settings": map[string]any{"owner": "{{ .env.TEAM }}"},
		"env":      map[string]any{"TEAM": "platform"},
	}
	paths := ExpandEvaluationPaths(input, [][]string{{"vars", "name"}})
	assert.ElementsMatch(t, [][]string{{"vars", "name"}, {"settings", "owner"}, {"env", "TEAM"}}, paths)
	selected, excluded := SplitEvaluationFields(input, paths)
	assert.NotContains(t, selected["vars"], "unused")
	RestoreEvaluationFields(selected, excluded)
	assert.Equal(t, input, selected)
	assert.Nil(t, ExpandEvaluationPaths(map[string]any{"vars": "{{ index . .key }}"}, [][]string{{"vars"}}))
	empty, _ := SplitEvaluationFields(input, [][]string{})
	assert.Empty(t, empty)
	all, _ := SplitEvaluationFields(input, nil)
	assert.Equal(t, input, all)
	assert.True(t, IsSectionRequired(nil, "vars"))
	assert.False(t, IsSectionRequired([]string{}, "vars"))
	assert.Equal(t, [][]string{{"vars", "name"}}, PathsForQuery(".vars.name"))
	assert.Nil(t, PathsForQuery(".vars | select(.enabled)"))
}

func TestRenderValuesPreservesStructure(t *testing.T) {
	input := map[string]any{"list": []any{"first", 42}, "nested": map[string]any{"second": "second"}}
	got, err := RenderValues(input, func(s string) (any, error) { return "rendered:" + s, nil })
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"list": []any{"rendered:first", 42}, "nested": map[string]any{"second": "rendered:second"}}, got)
	_, err = RenderValues(input, func(string) (any, error) { return nil, errUtils.ErrInvalidAuthConfig })
	require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
}

func TestEvaluationDemandCustomDelimiters(t *testing.T) {
	input := map[string]any{
		"vars":     map[string]any{"name": "[[ .settings.owner ]]", "unused": "!aws.account_id"},
		"settings": map[string]any{"owner": []any{"[[ .env.TEAM ]]"}},
		"env":      map[string]any{"TEAM": "platform"},
	}
	paths := ExpandEvaluationPaths(input, [][]string{{"vars", "name"}}, "[[", "]]")
	assert.ElementsMatch(t, [][]string{{"vars", "name"}, {"settings", "owner"}, {"env", "TEAM"}}, paths)
	assert.Equal(t, [][]string{}, ExpandEvaluationPaths(input, [][]string{}, "[[", "]]"))
	assert.Nil(t, ExpandEvaluationPaths(input, nil, "[[", "]]"))
	assert.Nil(t, ExpandEvaluationPaths(input, [][]string{{"vars", "name"}}, "[["))
	input["vars"] = map[string]any{"name": "[[ index . .key ]]"}
	assert.Nil(t, ExpandEvaluationPaths(input, [][]string{{"vars", "name"}}, "[[", "]]"))
}

func TestEvaluationDemandDelimiterOverrides(t *testing.T) {
	for _, tc := range []struct {
		name       string
		delimiters any
		template   string
		full       bool
	}{
		{"string pair", []string{"<<", ">>"}, "<< .settings.owner >>", false},
		{"YAML pair", []any{"<<", ">>"}, "<< .settings.owner >>", false},
		{"empty resets defaults", []any{}, "{{ .settings.owner }}", false},
		{"malformed pair", []any{42, ">>"}, "<< .settings.owner >>", true},
		{"dynamic delimiters", "[[ .vars.delimiters ]]", "<< .settings.owner >>", true},
		{"empty delimiter", []string{"", ">>"}, "<< .settings.owner >>", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := map[string]any{
				"vars": map[string]any{"name": tc.template},
				"settings": map[string]any{
					"owner":     "platform",
					"templates": map[string]any{"settings": map[string]any{"delimiters": tc.delimiters}},
				},
			}
			paths := ExpandEvaluationPaths(data, [][]string{{"vars", "name"}}, "[[", "]]")
			if tc.full {
				require.Nil(t, paths)
			} else {
				require.Equal(t, [][]string{{"vars", "name"}, {"settings", "owner"}}, paths)
			}
			require.Equal(t, [][]string{}, ExpandEvaluationPaths(data, [][]string{}, "[[", "]]"))
		})
	}
}
