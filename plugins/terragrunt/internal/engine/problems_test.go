package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// Every unsupported setting is rejected during discovery, so a bulk run can validate each selected
// unit before any infrastructure command runs.
func TestDiscoveryRejectsUnsupportedSettingsNamingTheKey(t *testing.T) {
	for name, test := range map[string]struct{ extra, key string }{
		"terraform_binary": {`terraform_binary = "tofu"`, "terraform_binary"},
		"errors":           {"errors {\n retry \"x\" {\n retryable_errors = [\".*\"]\n max_attempts = 2\n sleep_interval_sec = 1\n }\n}", "errors"},
		"exclude":          {"exclude {\n if = true\n actions = [\"plan\"]\n}", "exclude"},
		"iam":              {`iam_role = "arn:aws:iam::1:role/x"`, "iam_role"},
		"feature":          {"feature \"flag\" {\n default = true\n}", "feature"},
	} {
		t.Run(name, func(t *testing.T) {
			request := nativeUnit(t, test.extra)
			request.Phase = "discover"
			_, err := Resolve(t.Context(), request)
			problem := asProblem(t, err)
			require.Equal(t, model.CodeUnsupportedSetting, problem.Code)
			require.Equal(t, test.key, problem.Key)
			require.Equal(t, "terragrunt.hcl", filepath.Base(problem.File))
			require.True(t, problem.Safe)
			require.NotEmpty(t, problem.Hints)
		})
	}
}

func TestDiscoveryRejectsUnsupportedHooks(t *testing.T) {
	request := nativeUnit(t, "")
	body, err := os.ReadFile(request.Path)
	require.NoError(t, err)
	hooked := strings.Replace(string(body), "{ source", "{\n source", 1)
	hooked = strings.Replace(hooked, " }\n", "\n before_hook \"lint\" {\n commands = [\"plan\"]\n execute = [\"tflint\"]\n }\n}\n", 1)
	require.NoError(t, os.WriteFile(request.Path, []byte(hooked), 0o644))
	request.Phase = "discover"
	_, err = Resolve(t.Context(), request)
	problem := asProblem(t, err)
	require.Equal(t, model.CodeUnsupportedHook, problem.Code)
	require.Contains(t, problem.Key, "lint")
	require.Equal(t, "terragrunt.hcl", filepath.Base(problem.File))
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

// A missing local module is rejected during discovery, so bulk preflight stops before any unit
// runs, and the shown detail never repeats the evaluated source.
func TestMissingLocalSourceFailsDiscoveryWithoutShowingTheSource(t *testing.T) {
	for _, phase := range []string{"discover", ""} {
		t.Run("phase="+phase, func(t *testing.T) {
			request := nativeUnit(t, "")
			missing := filepath.Join(filepath.Dir(request.Path), "secret-token-value")
			source, err := json.Marshal(filepath.ToSlash(missing))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(request.Path, []byte("terraform { source = "+string(source)+" }\n"), 0o644))
			request.Phase = phase
			_, err = Resolve(t.Context(), request)
			problem := asProblem(t, err)
			require.Equal(t, model.CodeInvalidSource, problem.Code)
			require.Equal(t, "terraform.source", problem.Key)
			require.True(t, problem.Safe)
			require.NotContains(t, problem.Detail, "secret-token-value")
			require.NotContains(t, problem.Summary, "secret-token-value")
			require.ErrorIs(t, err, ErrUnsupportedExecution)
		})
	}
}
