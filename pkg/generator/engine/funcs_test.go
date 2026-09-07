package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestRenderAnswersListExpression_TopLevelKeys(t *testing.T) {
	p := NewProcessor()
	answers := map[string]interface{}{"environments": map[string]interface{}{"dev": nil, "staging": nil}}

	got, err := p.RenderAnswersListExpression("{{ collectKeys answers.environments }}", answers, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"dev", "staging"}, got)
}

func TestRenderAnswersListExpression_NestedKeysAcrossEnvironments(t *testing.T) {
	p := NewProcessor()
	answers := map[string]interface{}{
		"environments": map[string]interface{}{
			"dev":        map[string]interface{}{"regions": map[string]interface{}{"us-east-1": nil}},
			"staging":    map[string]interface{}{"regions": map[string]interface{}{"us-east-1": nil}},
			"production": map[string]interface{}{"regions": map[string]interface{}{"us-east-1": nil, "us-west-2": nil}},
		},
	}

	got, err := p.RenderAnswersListExpression(`{{ collectKeys answers.environments "regions" }}`, answers, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"us-east-1", "us-west-2"}, got)
}

func TestRenderAnswersListExpression_EmptyResultYieldsEmptySlice(t *testing.T) {
	p := NewProcessor()
	answers := map[string]interface{}{"environments": map[string]interface{}{}}

	got, err := p.RenderAnswersListExpression("{{ collectKeys answers.environments }}", answers, nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestRenderAnswersListExpression_InvalidTemplateErrors(t *testing.T) {
	p := NewProcessor()

	_, err := p.RenderAnswersListExpression("{{ collectKeys ", map[string]interface{}{}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldExpressionFailed)
}

func TestRenderAnswersListExpression_ExecutionErrorPropagates(t *testing.T) {
	p := NewProcessor()

	// answers.environments is a string, not a map, so collectKeys must fail at
	// execution time rather than parse time.
	_, err := p.RenderAnswersListExpression("{{ collectKeys answers.environments }}", map[string]interface{}{"environments": "dev,staging"}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldExpressionFailed)
}

// TestRenderAnswersListExpression_CustomDelimiters proves a template
// expression is detected/rendered using the scaffold's own spec.delimiters
// override (e.g. "[[" / "]]"), not a hardcoded "{{" / "}}", matching how
// target: and file content rendering already honor it via
// ProcessTemplateWithDelimiters.
func TestRenderAnswersListExpression_CustomDelimiters(t *testing.T) {
	p := NewProcessor()
	answers := map[string]interface{}{"environments": map[string]interface{}{"dev": nil, "staging": nil}}

	got, err := p.RenderAnswersListExpression("[[ collectKeys answers.environments ]]", answers, []string{"[[", "]]"})
	require.NoError(t, err)
	assert.Equal(t, []string{"dev", "staging"}, got)
}

// TestRenderAnswersListExpression_EmptyLeftDelimiterFallsBackToDefault
// proves a malformed delimiter pair with an empty left side (e.g. ["",
// "]]"]) falls back to the full "{{"/"}}" default rather than producing a
// mismatched half-custom/half-default pair (text/template.Delims treats an
// empty argument as "use the default" for that side only).
func TestRenderAnswersListExpression_EmptyLeftDelimiterFallsBackToDefault(t *testing.T) {
	p := NewProcessor()
	answers := map[string]interface{}{"environments": map[string]interface{}{"dev": nil, "staging": nil}}

	got, err := p.RenderAnswersListExpression("{{ collectKeys answers.environments }}", answers, []string{"", "]]"})
	require.NoError(t, err)
	assert.Equal(t, []string{"dev", "staging"}, got)
}

// TestRenderAnswersListExpression_EmptyRightDelimiterFallsBackToDefault
// mirrors TestRenderAnswersListExpression_EmptyLeftDelimiterFallsBackToDefault
// for an empty right delimiter (e.g. ["[[", ""]).
func TestRenderAnswersListExpression_EmptyRightDelimiterFallsBackToDefault(t *testing.T) {
	p := NewProcessor()
	answers := map[string]interface{}{"environments": map[string]interface{}{"dev": nil, "staging": nil}}

	got, err := p.RenderAnswersListExpression("{{ collectKeys answers.environments }}", answers, []string{"[[", ""})
	require.NoError(t, err)
	assert.Equal(t, []string{"dev", "staging"}, got)
}

func TestParseExpressionResult(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []string
		wantErr bool
	}{
		{name: "list", in: `["dev","staging","production"]`, want: []string{"dev", "staging", "production"}},
		{name: "empty list", in: "[]", want: []string{}},
		{name: "value with a space", in: `["us east","dev"]`, want: []string{"us east", "dev"}},
		{name: "value with a comma and quotes", in: `["team, \"platform\""]`, want: []string{`team, "platform"`}},
		{name: "not an array", in: `"dev"`, wantErr: true},
		{name: "non-string element", in: `["dev",5]`, wantErr: true},
		{name: "malformed json", in: `[dev]`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseExpressionResult(tt.in, "irrelevant")
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrScaffoldExpressionFailed)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRenderAnswersListExpression_WhitespaceValueSurvives is the real,
// end-to-end regression test for the bug the toJson wrap exists to fix:
// unlike a mocked AxisRenderer, this exercises the actual renderer, so a
// regression back to text/template's default "[a b c]" slice formatting
// (which splits "us east" into two values) would fail this test.
func TestRenderAnswersListExpression_WhitespaceValueSurvives(t *testing.T) {
	p := NewProcessor()
	answers := map[string]interface{}{"environments": map[string]interface{}{"us east": nil, "dev": nil}}

	got, err := p.RenderAnswersListExpression("{{ collectKeys answers.environments }}", answers, nil)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"us east", "dev"}, got)
}

// TestRenderAnswersListExpression_CommaAndQuotesSurvive covers the other
// values CodeRabbit's review asked for explicit coverage of.
func TestRenderAnswersListExpression_CommaAndQuotesSurvive(t *testing.T) {
	p := NewProcessor()
	answers := map[string]interface{}{"teams": map[string]interface{}{`team, "platform"`: nil}}

	got, err := p.RenderAnswersListExpression("{{ collectKeys answers.teams }}", answers, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{`team, "platform"`}, got)
}

// TestRenderAnswersListExpression_CustomDelimitersWithTrimMarkers proves the
// AST-based toJson append (not string/delimiter manipulation) survives Go's
// whitespace-trim markers at the action's boundary.
func TestRenderAnswersListExpression_CustomDelimitersWithTrimMarkers(t *testing.T) {
	p := NewProcessor()
	answers := map[string]interface{}{"environments": map[string]interface{}{"dev": nil, "staging": nil}}

	got, err := p.RenderAnswersListExpression("[[- collectKeys answers.environments -]]", answers, []string{"[[", "]]"})
	require.NoError(t, err)
	assert.Equal(t, []string{"dev", "staging"}, got)
}

func TestRenderAnswersListExpression_RejectsSurroundingText(t *testing.T) {
	p := NewProcessor()

	_, err := p.RenderAnswersListExpression("prefix {{ collectKeys answers.environments }}", map[string]interface{}{}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldExpressionFailed)
}

func TestRenderAnswersListExpression_RejectsMultipleActions(t *testing.T) {
	p := NewProcessor()

	_, err := p.RenderAnswersListExpression("{{ collectKeys answers.a }}{{ collectKeys answers.b }}", map[string]interface{}{}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldExpressionFailed)
}

func TestRenderAnswersListExpression_RejectsControlStructures(t *testing.T) {
	p := NewProcessor()

	_, err := p.RenderAnswersListExpression(`{{ range collectKeys answers.environments }}{{ . }}{{ end }}`, map[string]interface{}{}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldExpressionFailed)
}

func TestRenderAnswersListExpression_RejectsVariableAssignment(t *testing.T) {
	p := NewProcessor()

	_, err := p.RenderAnswersListExpression("{{ $x := collectKeys answers.environments }}", map[string]interface{}{}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldExpressionFailed)
}

// TestRenderAnswersListExpression_SplitListSurvivesWhitespace proves the fix
// generalizes to any Sprig/Gomplate function composed into an answers-list
// expression, not just collectKeys.
func TestRenderAnswersListExpression_SplitListSurvivesWhitespace(t *testing.T) {
	p := NewProcessor()
	answers := map[string]interface{}{"environments_csv": "us east,dev"}

	got, err := p.RenderAnswersListExpression(`{{ splitList "," answers.environments_csv }}`, answers, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"us east", "dev"}, got)
}

// TestRenderAnswersListExpression_InvalidResultType proves a computed
// expression that doesn't resolve to a list of strings fails clearly
// instead of being silently coerced.
func TestRenderAnswersListExpression_InvalidResultType(t *testing.T) {
	p := NewProcessor()

	_, err := p.RenderAnswersListExpression("{{ 5 }}", map[string]interface{}{}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldExpressionFailed)
}
