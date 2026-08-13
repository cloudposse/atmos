package engine

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestExpandMatrix_NoAxes(t *testing.T) {
	rows, err := ExpandMatrix(nil, map[string]interface{}{}, nil, nil)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Empty(t, rows[0])
}

func TestExpandMatrix_SingleLiteralAxis(t *testing.T) {
	matrix := map[string]any{"environment": []string{"dev", "staging", "production"}}

	rows, err := ExpandMatrix(matrix, map[string]interface{}{}, nil, nil)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, map[string]string{"environment": "dev"}, rows[0])
	assert.Equal(t, map[string]string{"environment": "staging"}, rows[1])
	assert.Equal(t, map[string]string{"environment": "production"}, rows[2])
}

func TestExpandMatrix_CartesianProductSortedByAxis(t *testing.T) {
	matrix := map[string]any{
		"environment": []string{"dev", "production"},
		"region":      []string{"us-east-1", "us-west-2"},
	}

	rows, err := ExpandMatrix(matrix, map[string]interface{}{}, nil, nil)
	require.NoError(t, err)
	require.Len(t, rows, 4)

	// Axes expand in sorted order (environment before region), and each
	// axis's own values are visited in declaration order, deterministically.
	assert.Equal(t, map[string]string{"environment": "dev", "region": "us-east-1"}, rows[0])
	assert.Equal(t, map[string]string{"environment": "dev", "region": "us-west-2"}, rows[1])
	assert.Equal(t, map[string]string{"environment": "production", "region": "us-east-1"}, rows[2])
	assert.Equal(t, map[string]string{"environment": "production", "region": "us-west-2"}, rows[3])
}

func TestExpandMatrix_DynamicAxisFromAnswers(t *testing.T) {
	matrix := map[string]any{"environment": "answers.environments"}
	answers := map[string]interface{}{"environments": []interface{}{"dev", "staging"}}

	rows, err := ExpandMatrix(matrix, answers, nil, nil)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "dev", rows[0]["environment"])
	assert.Equal(t, "staging", rows[1]["environment"])
}

func TestExpandMatrix_DynamicAxisEmptyAnswerYieldsZeroRows(t *testing.T) {
	matrix := map[string]any{"environment": "answers.environments"}
	answers := map[string]interface{}{"environments": []interface{}{}}

	rows, err := ExpandMatrix(matrix, answers, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

// TestExpandMatrix_AxisErrors consolidates every ExpandMatrix error path
// (an invalid axis type, a dynamic axis source that isn't found/isn't
// list-shaped, and resolveMatrixAxisFromAnswers's dot-path walk) into one
// table-driven test, per this repo's table-driven-test convention.
func TestExpandMatrix_AxisErrors(t *testing.T) {
	tests := []struct {
		name    string
		matrix  map[string]any
		answers map[string]interface{}
		wantErr error
	}{
		{
			name:    "dynamic axis missing answers prefix",
			matrix:  map[string]any{"environment": "environments"},
			answers: map[string]interface{}{},
			wantErr: errUtils.ErrScaffoldMatrixAxisInvalid,
		},
		{
			name:    "dynamic axis source not found",
			matrix:  map[string]any{"environment": "answers.missing"},
			answers: map[string]interface{}{},
			wantErr: errUtils.ErrScaffoldMatrixSourceNotFound,
		},
		{
			name:    "dynamic axis source not list",
			matrix:  map[string]any{"environment": "answers.environments"},
			answers: map[string]interface{}{"environments": "dev,staging"},
			wantErr: errUtils.ErrScaffoldMatrixSourceNotList,
		},
		{
			name:    "axis invalid type",
			matrix:  map[string]any{"environment": 5},
			answers: map[string]interface{}{},
			wantErr: errUtils.ErrScaffoldMatrixAxisInvalid,
		},
		{
			// Multi-segment dot-path: the first segment ("cloud") resolves
			// to a map, so the loop walks a second iteration before failing
			// to find "regions" -- previously uncovered, since the
			// single-segment case above only exercises one loop iteration.
			name:   "multi-segment dot-path source not found",
			matrix: map[string]any{"region": "answers.cloud.regions"},
			answers: map[string]interface{}{
				"cloud": map[string]interface{}{"provider": "aws"},
			},
			wantErr: errUtils.ErrScaffoldMatrixSourceNotFound,
		},
		{
			// Intermediate segment resolves to a non-map scalar: "cloud" is
			// a plain string, not a map, so the second segment ("regions")
			// can't be walked into it.
			name:   "intermediate path segment not a map",
			matrix: map[string]any{"region": "answers.cloud.regions"},
			answers: map[string]interface{}{
				"cloud": "aws",
			},
			wantErr: errUtils.ErrScaffoldMatrixSourceNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ExpandMatrix(tt.matrix, tt.answers, nil, nil)
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.wantErr), err)
		})
	}
}

func TestExpandMatrix_TemplateExpressionAxis(t *testing.T) {
	matrix := map[string]any{"environment": "{{ collectKeys answers.environments }}"}
	answers := map[string]interface{}{"environments": map[string]interface{}{"dev": nil, "staging": nil}}

	var gotExpr string
	var gotDelimiters []string
	render := func(expr string, a map[string]interface{}, delimiters []string) ([]string, error) {
		gotExpr = expr
		gotDelimiters = delimiters
		assert.Equal(t, answers, a)
		return []string{"dev", "staging"}, nil
	}

	rows, err := ExpandMatrix(matrix, answers, render, nil)
	require.NoError(t, err)
	assert.Equal(t, "{{ collectKeys answers.environments }}", gotExpr)
	assert.Equal(t, []string{"{{", "}}"}, gotDelimiters)
	require.Len(t, rows, 2)
	assert.Equal(t, "dev", rows[0]["environment"])
	assert.Equal(t, "staging", rows[1]["environment"])
}

// TestExpandMatrix_TemplateExpressionAxisValueContainingWhitespace proves a
// computed axis value that itself contains whitespace survives Cartesian
// product expansion as one intact value -- not split, not truncated. The
// render func here stands in for RenderMatrixAxisExpression (already
// covered directly by TestRenderMatrixAxisExpression_ValueContainingWhitespace
// in funcs_test.go); this test's job is to prove ExpandMatrix itself doesn't
// re-split or otherwise mangle a renderer's returned values on the way into
// a row.
func TestExpandMatrix_TemplateExpressionAxisValueContainingWhitespace(t *testing.T) {
	matrix := map[string]any{"environment": "{{ collectKeys answers.environments }}"}
	render := func(string, map[string]interface{}, []string) ([]string, error) {
		return []string{"us east", "dev"}, nil
	}

	rows, err := ExpandMatrix(matrix, map[string]interface{}{}, render, nil)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "us east", rows[0]["environment"])
	assert.Equal(t, "dev", rows[1]["environment"])
}

// TestExpandMatrix_TemplateExpressionAxisCustomDelimiters proves a matrix
// axis expression is detected as a template expression using the scaffold's
// own custom delimiters ("[["/"]]"), not a hardcoded "{{"/"}}" -- without
// this, an expression written with custom delimiters would fall through to
// resolveMatrixAxisFromAnswers and fail because it doesn't start with
// "answers.".
func TestExpandMatrix_TemplateExpressionAxisCustomDelimiters(t *testing.T) {
	matrix := map[string]any{"environment": "[[ collectKeys answers.environments ]]"}
	answers := map[string]interface{}{"environments": map[string]interface{}{"dev": nil, "staging": nil}}

	var gotDelimiters []string
	render := func(expr string, a map[string]interface{}, delimiters []string) ([]string, error) {
		gotDelimiters = delimiters
		assert.Equal(t, "[[ collectKeys answers.environments ]]", expr)
		return []string{"dev", "staging"}, nil
	}

	rows, err := ExpandMatrix(matrix, answers, render, []string{"[[", "]]"})
	require.NoError(t, err)
	assert.Equal(t, []string{"[[", "]]"}, gotDelimiters)
	require.Len(t, rows, 2)
	assert.Equal(t, "dev", rows[0]["environment"])
	assert.Equal(t, "staging", rows[1]["environment"])
}

// TestExpandMatrix_EmptyLeftDelimiterFallsBackToDefault proves a malformed
// delimiter pair with an empty left side (e.g. ["", "]]"]) is rejected as a
// custom pair and falls back to the full "{{"/"}}" default, rather than
// letting an empty delimiters[0] make every axis value match
// strings.Contains(v, "") in resolveMatrixAxis.
func TestExpandMatrix_EmptyLeftDelimiterFallsBackToDefault(t *testing.T) {
	matrix := map[string]any{"environment": "{{ collectKeys answers.environments }}"}
	answers := map[string]interface{}{"environments": map[string]interface{}{"dev": nil, "staging": nil}}

	var gotDelimiters []string
	render := func(expr string, a map[string]interface{}, delimiters []string) ([]string, error) {
		gotDelimiters = delimiters
		return []string{"dev", "staging"}, nil
	}

	rows, err := ExpandMatrix(matrix, answers, render, []string{"", "]]"})
	require.NoError(t, err)
	assert.Equal(t, []string{"{{", "}}"}, gotDelimiters)
	require.Len(t, rows, 2)
	assert.Equal(t, "dev", rows[0]["environment"])
	assert.Equal(t, "staging", rows[1]["environment"])
}

// TestExpandMatrix_EmptyRightDelimiterFallsBackToDefault mirrors
// TestExpandMatrix_EmptyLeftDelimiterFallsBackToDefault for an empty right
// delimiter (e.g. ["[[", ""]).
func TestExpandMatrix_EmptyRightDelimiterFallsBackToDefault(t *testing.T) {
	matrix := map[string]any{"environment": "{{ collectKeys answers.environments }}"}
	answers := map[string]interface{}{"environments": map[string]interface{}{"dev": nil, "staging": nil}}

	var gotDelimiters []string
	render := func(expr string, a map[string]interface{}, delimiters []string) ([]string, error) {
		gotDelimiters = delimiters
		return []string{"dev", "staging"}, nil
	}

	rows, err := ExpandMatrix(matrix, answers, render, []string{"[[", ""})
	require.NoError(t, err)
	assert.Equal(t, []string{"{{", "}}"}, gotDelimiters)
	require.Len(t, rows, 2)
	assert.Equal(t, "dev", rows[0]["environment"])
	assert.Equal(t, "staging", rows[1]["environment"])
}

func TestExpandMatrix_TemplateExpressionAxisWithoutRendererErrors(t *testing.T) {
	matrix := map[string]any{"environment": "{{ collectKeys answers.environments }}"}

	_, err := ExpandMatrix(matrix, map[string]interface{}{}, nil, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrScaffoldMatrixExpressionFailed), err)
}

func TestExpandMatrix_TemplateExpressionAxisPropagatesRenderError(t *testing.T) {
	matrix := map[string]any{"environment": "{{ collectKeys answers.environments }}"}
	renderErr := errors.New("boom")
	render := func(string, map[string]interface{}, []string) ([]string, error) {
		return nil, renderErr
	}

	_, err := ExpandMatrix(matrix, map[string]interface{}{}, render, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, renderErr), err)
}
