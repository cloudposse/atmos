package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
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

// A diagnostic without an expression is safe only when it points at a source range, as parser and
// schema diagnostics do. A conversion diagnostic can lack an expression and still quote a value.
func TestDiagnosticProblemTrustsOnlyParserAndSchemaDiagnostics(t *testing.T) {
	located := &hcl.Range{Filename: "terragrunt.hcl", Start: hcl.Pos{Line: 3, Column: 5}}
	for _, test := range []struct {
		name       string
		diagnostic *hcl.Diagnostic
		safe       bool
	}{
		{"parser or schema diagnostic", &hcl.Diagnostic{Severity: hcl.DiagError, Summary: "Unsupported argument", Detail: "An argument named \"x\" is not expected here.", Subject: located}, true},
		{"conversion diagnostic without an expression", &hcl.Diagnostic{Severity: hcl.DiagError, Summary: "Unsuitable value", Detail: "Unsuitable value: hunter2", Subject: located, EvalContext: &hcl.EvalContext{}}, false},
		{"diagnostic without a source range", &hcl.Diagnostic{Severity: hcl.DiagError, Summary: "Unsuitable value", Detail: "Unsuitable value: hunter2"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			problem := diagnosticProblem(hcl.Diagnostics{test.diagnostic}, "terragrunt.hcl", test.diagnostic)
			require.Equal(t, test.safe, problem.Safe)
			if !test.safe {
				require.NotEqual(t, model.CodeSyntaxError, problem.Code)
			}
		})
	}
}

// A local `dir//subdir` module source is checked during discovery like a plain local source: both the
// copied tree and the requested subdirectory must exist, and an existing pair passes.
func TestLocalSubdirectorySourceMustExistDuringDiscovery(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
		valid  bool
	}{
		{"missing copied tree", "./absent//app", false},
		{"missing subdirectory", "./modules//absent", false},
		{"subdirectory is a file", "./modules//file.tf", false},
		{"existing subdirectory", "./modules//app", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := nativeUnit(t, "")
			root := filepath.Dir(request.Path)
			require.NoError(t, os.MkdirAll(filepath.Join(root, "modules", "app"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(root, "modules", "file.tf"), nil, 0o644))
			require.NoError(t, os.WriteFile(request.Path, []byte("terraform { source = \""+test.source+"\" }\n"), 0o644))
			request.Phase = "discover"
			_, err := Resolve(t.Context(), request)
			if test.valid {
				require.NoError(t, err)
				return
			}
			problem := asProblem(t, err)
			require.Equal(t, model.CodeInvalidSource, problem.Code)
			require.Equal(t, "The module `source` is not an existing local directory", problem.Summary)
			require.True(t, problem.Safe)
			require.ErrorIs(t, err, ErrUnsupportedExecution)
		})
	}
}

func TestGeneratedPathDiagnosticsDoNotExposeEvaluatedSecrets(t *testing.T) {
	const secret = "private-evaluated-target"
	for _, extra := range []string{
		`generate "private" {
 path = "../${get_env("PRIVATE_TARGET")}"
 if_exists = "overwrite"
 contents = ""
}`,
		`generate "first" {
 path = get_env("PRIVATE_TARGET")
 if_exists = "overwrite"
 contents = ""
}
generate "second" {
 path = get_env("PRIVATE_TARGET")
 if_exists = "overwrite"
 contents = ""
}`,
		`remote_state {
 backend = "local"
 config = {}
 generate = { path = "../${get_env("PRIVATE_TARGET")}", if_exists = "overwrite" }
}`,
		`generate "first" {
 path = get_env("PRIVATE_TARGET")
 if_exists = "overwrite"
 contents = ""
}
remote_state {
 backend = "local"
 config = {}
 generate = { path = get_env("PRIVATE_TARGET"), if_exists = "overwrite" }
}`,
	} {
		request := nativeUnit(t, extra)
		request.Environment["PRIVATE_TARGET"] = secret
		_, err := Resolve(t.Context(), request)
		problem := asProblem(t, err)
		require.True(t, problem.Safe)
		require.NotContains(t, problem.Error(), secret)
		require.NotContains(t, problem.Detail, secret)
	}
}

func TestUnsupportedSourceDiagnosticsDoNotExposeEvaluatedText(t *testing.T) {
	require.NoError(t, validateRemoteSource("tfr://registry.example.test/module"))
	for _, source := range []string{"private-credential://module", "private-credential", "unsupported::private-credential/module"} {
		problem := asProblem(t, validateRemoteSource(source))
		require.True(t, problem.Safe)
		require.NotContains(t, problem.Error(), "private-credential")
		require.NotContains(t, problem.Detail, "private-credential")
		require.Contains(t, problem.Hints[0], "tfr://", "L7 supports registry sources")
	}
}
