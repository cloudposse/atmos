package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestKeysFunc_TopLevel(t *testing.T) {
	m := map[string]interface{}{"staging": nil, "dev": nil, "production": nil}

	got, err := keysFunc(m)
	require.NoError(t, err)
	assert.Equal(t, []string{"dev", "production", "staging"}, got)
}

func TestKeysFunc_NestedFlattensAndDedupes(t *testing.T) {
	m := map[string]interface{}{
		"dev": map[string]interface{}{
			"regions": map[string]interface{}{"us-east-1": nil},
		},
		"staging": map[string]interface{}{
			"regions": map[string]interface{}{"us-east-1": nil},
		},
		"production": map[string]interface{}{
			"regions": map[string]interface{}{"us-east-1": nil, "us-west-2": nil},
		},
	}

	got, err := keysFunc(m, "regions")
	require.NoError(t, err)
	assert.Equal(t, []string{"us-east-1", "us-west-2"}, got)
}

func TestKeysFunc_NestedSkipsEntriesMissingTheKey(t *testing.T) {
	m := map[string]interface{}{
		"dev":     map[string]interface{}{"regions": map[string]interface{}{"us-east-1": nil}},
		"staging": map[string]interface{}{"other": "value"},
		"weird":   "not a map at all",
	}

	got, err := keysFunc(m, "regions")
	require.NoError(t, err)
	assert.Equal(t, []string{"us-east-1"}, got)
}

func TestKeysFunc_NotAMapErrors(t *testing.T) {
	_, err := keysFunc("not a map")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldKeysFuncNotMap)
}

func TestRenderMatrixAxisExpression_TopLevelKeys(t *testing.T) {
	p := NewProcessor()
	answers := map[string]interface{}{"environments": map[string]interface{}{"dev": nil, "staging": nil}}

	got, err := p.RenderMatrixAxisExpression("{{ keys answers.environments }}", answers, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"dev", "staging"}, got)
}

func TestRenderMatrixAxisExpression_NestedKeysAcrossEnvironments(t *testing.T) {
	p := NewProcessor()
	answers := map[string]interface{}{
		"environments": map[string]interface{}{
			"dev":        map[string]interface{}{"regions": map[string]interface{}{"us-east-1": nil}},
			"staging":    map[string]interface{}{"regions": map[string]interface{}{"us-east-1": nil}},
			"production": map[string]interface{}{"regions": map[string]interface{}{"us-east-1": nil, "us-west-2": nil}},
		},
	}

	got, err := p.RenderMatrixAxisExpression(`{{ keys answers.environments "regions" }}`, answers, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"us-east-1", "us-west-2"}, got)
}

func TestRenderMatrixAxisExpression_EmptyResultYieldsEmptySlice(t *testing.T) {
	p := NewProcessor()
	answers := map[string]interface{}{"environments": map[string]interface{}{}}

	got, err := p.RenderMatrixAxisExpression("{{ keys answers.environments }}", answers, nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestRenderMatrixAxisExpression_InvalidTemplateErrors(t *testing.T) {
	p := NewProcessor()

	_, err := p.RenderMatrixAxisExpression("{{ keys ", map[string]interface{}{}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldMatrixExpressionFailed)
}

func TestRenderMatrixAxisExpression_ExecutionErrorPropagates(t *testing.T) {
	p := NewProcessor()

	// answers.environments is a string, not a map, so keys must fail at
	// execution time rather than parse time.
	_, err := p.RenderMatrixAxisExpression("{{ keys answers.environments }}", map[string]interface{}{"environments": "dev,staging"}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldMatrixExpressionFailed)
}

// TestRenderMatrixAxisExpression_CustomDelimiters proves a matrix axis
// expression is detected/rendered using the scaffold's own custom
// spec.delimiters override (e.g. "[[" / "]]"), not a hardcoded "{{" / "}}",
// matching how target: and file content rendering already honor it via
// ProcessTemplateWithDelimiters.
func TestRenderMatrixAxisExpression_CustomDelimiters(t *testing.T) {
	p := NewProcessor()
	answers := map[string]interface{}{"environments": map[string]interface{}{"dev": nil, "staging": nil}}

	got, err := p.RenderMatrixAxisExpression("[[ keys answers.environments ]]", answers, []string{"[[", "]]"})
	require.NoError(t, err)
	assert.Equal(t, []string{"dev", "staging"}, got)
}

// TestRenderMatrixAxisExpression_EmptyLeftDelimiterFallsBackToDefault
// proves a malformed delimiter pair with an empty left side (e.g. ["",
// "]]"]) falls back to the full "{{"/"}}" default rather than producing a
// mismatched half-custom/half-default pair (text/template.Delims treats an
// empty argument as "use the default" for that side only).
func TestRenderMatrixAxisExpression_EmptyLeftDelimiterFallsBackToDefault(t *testing.T) {
	p := NewProcessor()
	answers := map[string]interface{}{"environments": map[string]interface{}{"dev": nil, "staging": nil}}

	got, err := p.RenderMatrixAxisExpression("{{ keys answers.environments }}", answers, []string{"", "]]"})
	require.NoError(t, err)
	assert.Equal(t, []string{"dev", "staging"}, got)
}

// TestRenderMatrixAxisExpression_EmptyRightDelimiterFallsBackToDefault
// mirrors TestRenderMatrixAxisExpression_EmptyLeftDelimiterFallsBackToDefault
// for an empty right delimiter (e.g. ["[[", ""]).
func TestRenderMatrixAxisExpression_EmptyRightDelimiterFallsBackToDefault(t *testing.T) {
	p := NewProcessor()
	answers := map[string]interface{}{"environments": map[string]interface{}{"dev": nil, "staging": nil}}

	got, err := p.RenderMatrixAxisExpression("{{ keys answers.environments }}", answers, []string{"[[", ""})
	require.NoError(t, err)
	assert.Equal(t, []string{"dev", "staging"}, got)
}

func TestParseBracketedList(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "bracketed", in: "[dev staging production]", want: []string{"dev", "staging", "production"}},
		{name: "empty brackets", in: "[]", want: []string{}},
		{name: "plain whitespace separated", in: "dev staging", want: []string{"dev", "staging"}},
		{name: "surrounding whitespace", in: "  [dev]  \n", want: []string{"dev"}},
		{name: "empty string", in: "", want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseBracketedList(tt.in))
		})
	}
}
