package engine

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

func asProblem(t *testing.T, err error) *model.Problem {
	t.Helper()
	var problem *model.Problem
	require.ErrorAs(t, err, &problem)
	return problem
}

func TestSyntaxErrorNamesFileLineAndColumnAndStaysSafe(t *testing.T) {
	request := nativeUnit(t, "inputs = {\n  a = find_in_parent_folders(\"x\"\n}\n")
	request.Phase = "discover"
	_, err := Resolve(t.Context(), request)
	problem := asProblem(t, err)
	require.Equal(t, model.CodeSyntaxError, problem.Code)
	require.Equal(t, "terragrunt.hcl", filepath.Base(problem.File))
	require.Positive(t, problem.Line)
	require.Positive(t, problem.Column)
	require.True(t, problem.Safe, "parser diagnostics hold no evaluated values")
	require.Contains(t, problem.Summary, "Missing argument separator")
}

func TestEvaluationErrorKeepsEvaluatedTextHidden(t *testing.T) {
	request := nativeUnit(t, "inputs = {\n  a = tonumber(\"hunter2\")\n}\n")
	_, err := Resolve(t.Context(), request)
	problem := asProblem(t, err)
	require.False(t, problem.Safe)
	require.NotContains(t, problem.Summary, "hunter2")
	for _, hint := range problem.Hints {
		require.NotContains(t, hint, "hunter2")
	}
	require.Positive(t, problem.Line)
}

func TestMissingDependencyTargetIsNamed(t *testing.T) {
	request := nativeUnit(t, "dependency \"gone\" {\n config_path = \"./gone\"\n}\n")
	request.Phase = "discover"
	_, err := Resolve(t.Context(), request)
	problem := asProblem(t, err)
	require.Equal(t, model.CodeDependency, problem.Code)
	require.Contains(t, problem.Detail, "gone")
	require.Equal(t, "dependency", problem.Key)
}

func TestProblemForPassesCancellationAndWrapsUnknownErrors(t *testing.T) {
	wrapped := problemFor(errors.New("secret detail"), filepath.Join("a", "b.hcl"))
	problem := asProblem(t, wrapped)
	require.False(t, problem.Safe)
	require.Equal(t, model.CodeEvaluationError, problem.Code)
	require.NoError(t, problemFor(nil, "f"))
	require.ErrorIs(t, problemFor(errContextCanceled(), "f"), errContextCanceled())
}

func errContextCanceled() error { return context.Canceled }
