package engine

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestExpandMatrix_NoAxes(t *testing.T) {
	rows, err := ExpandMatrix(nil, map[string]interface{}{}, nil)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Empty(t, rows[0])
}

func TestExpandMatrix_SingleLiteralAxis(t *testing.T) {
	matrix := map[string]any{"environment": []string{"dev", "staging", "production"}}

	rows, err := ExpandMatrix(matrix, map[string]interface{}{}, nil)
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

	rows, err := ExpandMatrix(matrix, map[string]interface{}{}, nil)
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

	rows, err := ExpandMatrix(matrix, answers, nil)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "dev", rows[0]["environment"])
	assert.Equal(t, "staging", rows[1]["environment"])
}

func TestExpandMatrix_DynamicAxisEmptyAnswerYieldsZeroRows(t *testing.T) {
	matrix := map[string]any{"environment": "answers.environments"}
	answers := map[string]interface{}{"environments": []interface{}{}}

	rows, err := ExpandMatrix(matrix, answers, nil)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestExpandMatrix_DynamicAxisMissingAnswersPrefix(t *testing.T) {
	matrix := map[string]any{"environment": "environments"}

	_, err := ExpandMatrix(matrix, map[string]interface{}{}, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrScaffoldMatrixAxisInvalid), err)
}

func TestExpandMatrix_DynamicAxisSourceNotFound(t *testing.T) {
	matrix := map[string]any{"environment": "answers.missing"}

	_, err := ExpandMatrix(matrix, map[string]interface{}{}, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrScaffoldMatrixSourceNotFound), err)
}

func TestExpandMatrix_DynamicAxisSourceNotList(t *testing.T) {
	matrix := map[string]any{"environment": "answers.environments"}
	answers := map[string]interface{}{"environments": "dev,staging"}

	_, err := ExpandMatrix(matrix, answers, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrScaffoldMatrixSourceNotList), err)
}

func TestExpandMatrix_AxisInvalidType(t *testing.T) {
	matrix := map[string]any{"environment": 5}

	_, err := ExpandMatrix(matrix, map[string]interface{}{}, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrScaffoldMatrixAxisInvalid), err)
}

func TestExpandMatrix_TemplateExpressionAxis(t *testing.T) {
	matrix := map[string]any{"environment": "{{ keys answers.environments }}"}
	answers := map[string]interface{}{"environments": map[string]interface{}{"dev": nil, "staging": nil}}

	var gotExpr string
	render := func(expr string, a map[string]interface{}) ([]string, error) {
		gotExpr = expr
		assert.Equal(t, answers, a)
		return []string{"dev", "staging"}, nil
	}

	rows, err := ExpandMatrix(matrix, answers, render)
	require.NoError(t, err)
	assert.Equal(t, "{{ keys answers.environments }}", gotExpr)
	require.Len(t, rows, 2)
	assert.Equal(t, "dev", rows[0]["environment"])
	assert.Equal(t, "staging", rows[1]["environment"])
}

func TestExpandMatrix_TemplateExpressionAxisWithoutRendererErrors(t *testing.T) {
	matrix := map[string]any{"environment": "{{ keys answers.environments }}"}

	_, err := ExpandMatrix(matrix, map[string]interface{}{}, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrScaffoldMatrixExpressionFailed), err)
}

func TestExpandMatrix_TemplateExpressionAxisPropagatesRenderError(t *testing.T) {
	matrix := map[string]any{"environment": "{{ keys answers.environments }}"}
	renderErr := errors.New("boom")
	render := func(string, map[string]interface{}) ([]string, error) {
		return nil, renderErr
	}

	_, err := ExpandMatrix(matrix, map[string]interface{}{}, render)
	require.Error(t, err)
	assert.True(t, errors.Is(err, renderErr), err)
}
