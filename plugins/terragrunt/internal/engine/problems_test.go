package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
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

func TestMissingDependencyTargetStaysHidden(t *testing.T) {
	request := nativeUnit(t, "dependency \"gone\" {\n config_path = get_env(\"DEPENDENCY_TARGET\")\n}\n")
	request.Phase = "discover"
	request.Environment["DEPENDENCY_TARGET"] = "./secret-dependency-target"
	_, err := Resolve(t.Context(), request)
	problem := asProblem(t, err)
	require.Equal(t, model.CodeDependency, problem.Code)
	require.True(t, problem.Safe)
	require.NotContains(t, problem.Error(), "secret-dependency-target")
	expectedFile, err := filepath.EvalSymlinks(request.Path)
	require.NoError(t, err)
	require.Equal(t, expectedFile, problem.File)
	require.NotEmpty(t, problem.Hints)
	require.ErrorIs(t, problem, os.ErrNotExist)
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

// statFailureFs fails every Stat with a fixed error, as a directory without search permission would.
type statFailureFs struct {
	afero.Fs
	err error
}

func (f statFailureFs) Stat(string) (os.FileInfo, error) { return nil, f.err }

// A local module source that is missing or a file is an invalid source; any other filesystem failure
// is reported as unreadable and keeps its cause instead of being mistaken for a missing directory.
func TestLocalSourceProblemDistinguishesFilesystemFailures(t *testing.T) {
	memory := afero.NewMemMapFs()
	require.NoError(t, memory.MkdirAll("/project/module", 0o755))
	require.NoError(t, afero.WriteFile(memory, "/project/file.tf", nil, 0o644))
	require.Nil(t, localSourceProblem(memory, "/project/terragrunt.hcl", "./module", "/project/module"))

	for _, resolved := range []string{"/project/absent", "/project/file.tf"} {
		problem := localSourceProblem(memory, "/project/terragrunt.hcl", "./x", resolved)
		require.NotNil(t, problem, resolved)
		require.Equal(t, "The module `source` is not an existing local directory", problem.Summary, resolved)
	}

	problem := localSourceProblem(statFailureFs{Fs: memory, err: fs.ErrPermission}, "/project/terragrunt.hcl", "./module", "/project/module")
	require.NotNil(t, problem)
	require.Equal(t, model.CodeInvalidSource, problem.Code)
	require.Equal(t, "The module `source` directory cannot be read", problem.Summary)
	require.ErrorIs(t, problem, fs.ErrPermission, "the filesystem cause is preserved")
	require.True(t, problem.Safe)
	require.NotContains(t, problem.Detail, "/project", "the safe detail names no path")
}
