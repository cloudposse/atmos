package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveWithholdsTransformedSensitiveInputCommandOutput(t *testing.T) {
	for _, test := range []struct {
		name, expression string
		withheld         bool
	}{
		{"direct call", "sensitive(%s)", false},
		{"transform inside", "sensitive(upper(%s))", true},
		{"transform outside", "upper(sensitive(%s))", true},
		{"nested input", "{ key = sensitive(upper(%s)) }", true},
		{"merge overwrite", `merge({ token = sensitive(%s) }, { token = "replacement" })`, true},
		{"merge disjoint", `merge({ token = sensitive(%s) }, { public = "visible" })`, false},
		{"merge unknown keys", `merge({ token = sensitive(%s) }, jsondecode("{\"token\":\"replacement\"}"))`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			call := effectCommand(t, "raw-secret-73ea")
			request := nativeUnit(t, "inputs = { token = "+fmt.Sprintf(test.expression, call)+", public = upper(\"visible\") }\n")
			request.Environment[effectChildEnv] = "1"
			sink := NewLogSink("debug", request.BasePath)
			logger(t, sink).Debug("before raw-secret-73ea")
			result, err := Resolve(WithLogSink(t.Context(), sink), request)
			require.NoError(t, err)
			require.NotEmpty(t, result.SensitiveInputs)
			effects, err := os.ReadFile(filepath.Join(request.BasePath, "effects"))
			require.NoError(t, err, "the Go subprocess must actually execute")
			assert.Contains(t, string(effects), "raw-secret-73ea")
			entries := sink.Entries()
			require.NotEmpty(t, entries)
			for _, entry := range entries {
				assert.NotContains(t, entry.Message, "raw-secret-73ea")
			}
			if test.withheld {
				require.Len(t, entries, 1)
				assert.Contains(t, entries[0].Message, "withheld")
			} else {
				assert.Equal(t, "before <MASKED>", entries[0].Message)
			}
		})
	}
}

func TestApplyTimeReparseWithholdsTransformedSensitiveInput(t *testing.T) {
	request := nativeUnit(t, `inputs = { token = "plan-input", phase = get_terraform_command() }`)
	request.Environment[effectChildEnv] = "1"
	sink := NewLogSink("debug", request.BasePath)
	ctx := config.WithCaches(WithLogSink(t.Context(), sink))
	l, err := evaluationLogger(ctx, "debug")
	require.NoError(t, err)
	environment, err := evaluationEnvironment(ctx, &request, nil, request.Path)
	require.NoError(t, err)
	pctx := requestParsingContext(&request, request.Path)
	evaluator := unitEvaluator{logger: l, environment: environment, pctx: pctx, request: &request, path: request.Path}
	cfg, err := evaluator.parseUnit(ctx)
	require.NoError(t, err)
	body, err := os.ReadFile(request.Path)
	require.NoError(t, err)
	body = []byte(strings.ReplaceAll(string(body), `"plan-input"`, "sensitive(upper("+effectCommand(t, "apply-raw-secret")+"))"))
	require.NoError(t, os.WriteFile(request.Path, body, 0o600))
	l.Debug("before apply-raw-secret")
	require.NotContains(t, sink.Entries()[0].Message, "withheld")
	_, _, err = evaluator.unitExecution(ctx, cfg, &sensitivityScan{})
	require.NoError(t, err)
	entries := sink.Entries()
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].Message, "withheld")
	assert.NotContains(t, entries[0].Message, "apply-raw-secret")
}

func TestSensitiveInputsSelectExposedInclude(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "repo")
	unit := filepath.Join(root, "app", unitFile)
	parent := filepath.Join(root, "parent.hcl")
	for _, test := range []struct {
		name   string
		path   string
		expose string
		found  bool
	}{
		{"ancestor", `find_in_parent_folders("parent.hcl")`, "true", true},
		{"absolute", fmt.Sprintf("%q", filepath.ToSlash(parent)), "true", true},
		{"missing", `"missing.hcl"`, "true", false},
		{"hidden", `"parent.hcl"`, "false", false},
		{"unknown exposure", `"parent.hcl"`, "local.expose", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			require.NoError(t, fsys.MkdirAll(filepath.Dir(unit), 0o755))
			require.NoError(t, afero.WriteFile(fsys, parent, []byte(`locals { token = sensitive("private") }`), 0o644))
			content := fmt.Sprintf("include \"root\" {\n path = %s\n expose = %s\n}\ninputs = { token = include.root.locals.token }", test.path, test.expose)
			require.NoError(t, afero.WriteFile(fsys, unit, []byte(content), 0o644))
			paths := SensitiveInputs(fsys, []string{unit, parent}, map[string]any{"token": "private"})
			if test.found {
				require.Equal(t, [][]string{{"token"}}, paths)
			} else {
				require.Empty(t, paths)
			}
		})
	}
}

func TestApplyValidationFailureRetainsSensitiveFileReads(t *testing.T) {
	request := nativeUnit(t, "include \"extra\" {\n path = get_terraform_command() == \"apply\" ? \"${get_terragrunt_dir()}/apply.hcl\" : \"${get_terragrunt_dir()}/plan.hcl\"\n}\n")
	request.Environment[effectChildEnv] = "1"
	dir := filepath.Dir(request.Path)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plan.hcl"), []byte("inputs = {}\n"), 0o600))
	body := fmt.Sprintf("locals {\n token = sensitive(%s)\n args = get_terraform_cli_args()\n}\n", effectCommand(t, "apply-validation-secret"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "apply.hcl"), []byte(body), 0o600))
	sink := NewLogSink("debug", request.BasePath)
	logger(t, sink).Debug("before apply-validation-secret")
	_, err := Resolve(WithLogSink(t.Context(), sink), request)
	require.ErrorIs(t, err, ErrUnsupportedExecution)
	effects, err := os.ReadFile(filepath.Join(request.BasePath, "effects"))
	require.NoError(t, err)
	require.Contains(t, string(effects), "apply-validation-secret")
	entries := sink.Entries()
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Message, "withheld")
	require.NotContains(t, entries[0].Message, "apply-validation-secret")
}
