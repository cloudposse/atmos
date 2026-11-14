package terraform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestOptionalTflintVarFilesResolveAtHookExecution(t *testing.T) {
	moduleDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "module.tfvars"), nil, 0o644))
	unitDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "unit.tfvars"), nil, 0o644))
	absolute := filepath.Join(unitDir, "unit.tfvars")
	arguments := []string{"--config", ".tflint.hcl", tflintOptionalVarFilePrefix + "module.tfvars", tflintOptionalVarFilePrefix + "missing.tfvars", tflintOptionalVarFilePrefix + absolute}
	require.Equal(t, []string{"--config", ".tflint.hcl", "--var-file=module.tfvars", "--var-file=" + absolute},
		resolveOptionalTflintVarFiles(arguments, moduleDir))
}

func TestTflintConfigLookupWaitsForHookExecution(t *testing.T) {
	unitDir := t.TempDir()
	arguments := []string{"--config", tflintConfigSearchPrefix + unitDir, "--chdir", "."}
	// Preparation can finish before a previous hook generates the config.
	config := filepath.Join(unitDir, ".tflint.hcl")
	require.NoError(t, os.WriteFile(config, []byte("plugin \"aws\" {}"), 0o644))
	resolved, err := resolveTflintHookArguments(arguments, t.TempDir(), nil)
	require.NoError(t, err)
	require.Equal(t, []string{"--config", config, "--chdir", "."}, resolved)
}

func TestTflintInputVariablesBindToTheFinalInputsAtHookExecution(t *testing.T) {
	// The provider's own value of count is overridden by the stack's, so the hook must see the stack's.
	inputs := map[string]any{
		"count":  2,
		"name":   "web",
		"tags":   map[string]any{"env": "${prod}"},
		"zone":   "a",
		"nested": []any{"%{x}"},
	}
	arguments := []string{"--config", ".tflint.hcl", tflintInputVariablesMarker, "--var-file=extra.tfvars", "--minimum-failure-severity=error"}

	resolved, err := resolveTflintHookArguments(arguments, t.TempDir(), inputs)

	require.NoError(t, err)
	require.Equal(t, []string{
		"--config", ".tflint.hcl",
		"--var=count=2", "--var=name=web", "--var=nested=[\"%%{x}\"]", "--var=tags={\"env\":\"$${prod}\"}", "--var=zone=a",
		"--var-file=extra.tfvars", "--minimum-failure-severity=error",
	}, resolved)
	none, err := resolveTflintInputVariables([]string{tflintInputVariablesMarker}, nil)
	require.NoError(t, err)
	require.Empty(t, none, "no inputs bind no variables")
	_, err = resolveTflintInputVariables([]string{tflintInputVariablesMarker}, map[string]any{"bad": make(chan int)})
	require.ErrorIs(t, err, errUtils.ErrComponentHookFailed)
	require.ErrorContains(t, err, `"bad"`)
}

func TestTflintVariableValueRendering(t *testing.T) {
	text, err := tflintVariableValue("plain")
	require.NoError(t, err)
	require.Equal(t, "plain", text)
	text, err = tflintVariableValue(map[string]any{"a": []any{1, "${x}"}})
	require.NoError(t, err)
	require.Equal(t, `{"a":[1,"$${x}"]}`, text)
	text, err = tflintVariableValue(map[string]any{"literal": "%{ if true } ${x}"})
	require.NoError(t, err)
	expr, diags := hclsyntax.ParseExpression([]byte(text), "input", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), diags.Error())
	value, diags := expr.Value(nil)
	require.False(t, diags.HasErrors(), diags.Error())
	require.Equal(t, "%{ if true } ${x}", value.GetAttr("literal").AsString())
	_, err = tflintVariableValue(make(chan int))
	require.Error(t, err)
}
