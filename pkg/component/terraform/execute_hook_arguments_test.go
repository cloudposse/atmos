package terraform

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/execution"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
	"github.com/cloudposse/atmos/pkg/schema"
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

// Inputs classified as secret reach TFLint through its TF_VAR_ environment, never as --var arguments,
// which any local process could read from the process table.
func TestTflintSecretInputsStayOutOfArguments(t *testing.T) {
	hook := &schema.ComponentHook{Name: "lint", Execute: []string{"tflint", "--chdir", ".", tflintInputVariablesMarker}}
	info := hookInfo("plan", &schema.ComponentExecution{})
	info.ComponentVarsSection = map[string]any{"name": "web", "password": "tok-456", "keys": []any{"tok-789"}}
	info.TerraformSecretVarKeys, info.TerraformSecretVarKeysComputed = map[string]bool{"password": true, "keys": true}, true
	shell := newShellFixture(t, nil)
	runner := &hookRunner{executor: hookExecutor(shell), atmosConfig: &schema.AtmosConfiguration{}, info: info, phase: &executionPhase{command: "plan", componentPath: t.TempDir()}}

	_, err := runner.RunHook(context.Background(), execution.KindBefore, hook)

	require.NoError(t, err)
	require.Len(t, shell.calls, 1)
	call := shell.calls[0]
	require.Equal(t, []string{"--chdir", ".", "--var=name=web"}, call.args, "only public inputs are passed as arguments")
	for _, argument := range call.args {
		require.NotContains(t, argument, "tok-", "a secret value must not appear in the argument vector")
	}
	require.Contains(t, call.env, "TF_VAR_password=tok-456")
	require.Contains(t, call.env, `TF_VAR_keys=["tok-789"]`)
	require.False(t, slicesContainsPrefix(call.env, "TF_VAR_name="), "public inputs are not duplicated into the environment")
}

// The read-config and init-from-module hooks run before the invocation classifies its inputs. Until
// then every bound input reaches TFLint through its environment, and none through the arguments.
func TestTflintInputsStayOutOfArgumentsBeforeClassification(t *testing.T) {
	unit := t.TempDir()
	lint := func(command string) schema.ComponentHook {
		return schema.ComponentHook{Name: "lint", Commands: []string{command}, Execute: []string{"tflint", "--chdir", ".", tflintInputVariablesMarker}, WorkingDirectory: unit}
	}
	unclassified := func(config *schema.ComponentExecution) *schema.ConfigAndStacksInfo {
		info := hookInfo("plan", config)
		info.ComponentVarsSection = map[string]any{"name": "web", "password": "tok-456"}
		// The provider declared the input sensitive, but nothing has classified the inputs yet.
		info.ConfigurationSensitiveInputKeys = map[string]bool{"password": true}
		return info
	}
	requireEnvironmentOnly := func(t *testing.T, shell *shellFixture) {
		t.Helper()
		require.Len(t, shell.calls, 1)
		call := shell.calls[0]
		require.Equal(t, []string{"--chdir", "."}, call.args, "no input is bound as an argument before classification")
		for _, argument := range call.args {
			require.NotContains(t, argument, "tok-", "a secret value must not appear in the argument vector")
		}
		require.Contains(t, call.env, "TF_VAR_password=tok-456")
		require.Contains(t, call.env, "TF_VAR_name=web")
	}

	t.Run("read-config", func(t *testing.T) {
		config := &schema.ComponentExecution{AfterHooks: []schema.ComponentHook{lint("read-config")}}
		shell := newShellFixture(t, nil)
		prepared := &schema.PreparedComponentConfiguration{Execution: config}
		require.NoError(t, hookExecutor(shell).runReadConfigHooks(t.Context(), &schema.AtmosConfiguration{}, unclassified(config), prepared))
		requireEnvironmentOnly(t, shell)
	})

	t.Run("init-from-module", func(t *testing.T) {
		config := &schema.ComponentExecution{AfterHooks: []schema.ComponentHook{lint("init-from-module")}}
		shell := newShellFixture(t, nil)
		wrapper := hookExecutor(shell).initFromModuleWrapper(&schema.AtmosConfiguration{}, unclassified(config))
		require.NotNil(t, wrapper)
		require.NoError(t, wrapper(t.Context(), &source.Target{Dir: unit}, func(context.Context) error { return nil }))
		requireEnvironmentOnly(t, shell)
	})
}

// Once the invocation has classified its inputs, a public input is still bound as an argument and only
// the secret ones go through the environment. An invocation without secret inputs binds every input.
func TestTflintPublicInputsBindAsArgumentsAfterClassification(t *testing.T) {
	hook := &schema.ComponentHook{Name: "lint", Execute: []string{"tflint", "--chdir", ".", tflintInputVariablesMarker}}
	run := func(t *testing.T, sensitive map[string]bool) shellCall {
		t.Helper()
		info := hookInfo("plan", &schema.ComponentExecution{})
		info.ComponentVarsSection = map[string]any{"name": "web", "password": "tok-456"}
		info.ConfigurationSensitiveInputKeys = sensitive
		ComputeTerraformSecretVarKeys(info)
		shell := newShellFixture(t, nil)
		runner := &hookRunner{executor: hookExecutor(shell), atmosConfig: &schema.AtmosConfiguration{}, info: info, phase: &executionPhase{command: "plan", componentPath: t.TempDir()}}
		_, err := runner.RunHook(context.Background(), execution.KindBefore, hook)
		require.NoError(t, err)
		require.Len(t, shell.calls, 1)
		return shell.calls[0]
	}

	t.Run("classified with a secret input", func(t *testing.T) {
		call := run(t, map[string]bool{"password": true})
		require.Equal(t, []string{"--chdir", ".", "--var=name=web"}, call.args)
		require.Contains(t, call.env, "TF_VAR_password=tok-456")
		require.False(t, slicesContainsPrefix(call.env, "TF_VAR_name="), "a public input is not moved into the environment")
	})

	t.Run("classified without secret inputs", func(t *testing.T) {
		call := run(t, nil)
		require.Equal(t, []string{"--chdir", ".", "--var=name=web", "--var=password=tok-456"}, call.args, "a computed, empty classification is not an unknown one")
		require.False(t, slicesContainsPrefix(call.env, "TF_VAR_"))
	})
}

// A saved-plan apply restores a preparation without inputs, because the plan record stores none. A
// tflint hook then binds the public inputs of the reviewed variable file, which the working directory
// digest of the plan covers. Secret inputs are never written to that file, so they are not bound.
func TestTflintBindsReviewedVarFileInputsOnSavedPlanApply(t *testing.T) {
	config, info, execCtx, _, path := planFixture(t)
	info.ComponentSection = map[string]any{}
	varFile := ConstructTerraformComponentVarfilePath(config, info)
	require.NoError(t, os.WriteFile(varFile, []byte(`{"count":3,"id":12345678901234567890,"region":"eu","tags":{"env":"prod"}}`), 0o600))
	observeFixturePlan(t, config, info, execCtx, path)
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"plan", "-out", filepath.Base(path)}))
	info.SubCommand, info.UseTerraformPlan, info.PlanFile = "apply", true, path
	prepared, err := restoreConfigurationPlan(t.Context(), config, info)
	require.NoError(t, err)
	applyPreparedConfiguration(config, info, prepared)
	require.Empty(t, info.ComponentVarsSection, "the restored preparation carries no inputs")

	hook := &schema.ComponentHook{Name: "lint", Execute: []string{"tflint", "--chdir", ".", tflintInputVariablesMarker}}
	run := func(t *testing.T, info *schema.ConfigAndStacksInfo) (shellCall, error) {
		t.Helper()
		shell := newShellFixture(t, nil)
		runner := &hookRunner{executor: hookExecutor(shell), atmosConfig: config, info: info, phase: &executionPhase{command: "apply", componentPath: execCtx.workingDir}}
		_, err := runner.RunHook(context.Background(), execution.KindBefore, hook)
		if err != nil {
			return shellCall{}, err
		}
		require.Len(t, shell.calls, 1)
		return shell.calls[0], nil
	}

	t.Run("before classification every reviewed input uses the environment", func(t *testing.T) {
		call, err := run(t, info)
		require.NoError(t, err)
		require.Equal(t, []string{"--chdir", "."}, call.args)
		require.Contains(t, call.env, "TF_VAR_region=eu")
		require.Contains(t, call.env, "TF_VAR_id=12345678901234567890")
	})

	t.Run("after classification the reviewed inputs bind as arguments", func(t *testing.T) {
		classified := *info
		ComputeTerraformSecretVarKeys(&classified)
		call, err := run(t, &classified)
		require.NoError(t, err)
		require.Equal(t, []string{"--chdir", ".", "--var=count=3", "--var=id=12345678901234567890", "--var=region=eu", `--var=tags={"env":"prod"}`}, call.args)
		require.False(t, slicesContainsPrefix(call.env, "TF_VAR_"))
		require.Empty(t, classified.ComponentVarsSection, "the inputs Terraform applies are not changed")
		require.Nil(t, classified.TerraformSecretVarKeys, "the partition used for Terraform is not changed")
	})

	t.Run("a reviewed input the provider declares sensitive stays out of the arguments", func(t *testing.T) {
		classified := *info
		classified.ConfigurationSensitiveInputKeys = map[string]bool{"region": true}
		ComputeTerraformSecretVarKeys(&classified)
		call, err := run(t, &classified)
		require.NoError(t, err)
		require.NotContains(t, call.args, "--var=region=eu")
		require.Contains(t, call.args, "--var=count=3")
		require.Contains(t, call.env, "TF_VAR_region=eu")
		require.Nil(t, classified.TerraformSecretVarKeys, "the partition used for Terraform is not changed")
	})

	t.Run("an invocation that applies no saved plan never reads the variable file", func(t *testing.T) {
		direct := *info
		direct.UseTerraformPlan, direct.PlanFile = false, ""
		direct.ComponentVarsSection = map[string]any{"region": "us"}
		ComputeTerraformSecretVarKeys(&direct)
		call, err := run(t, &direct)
		require.NoError(t, err)
		require.Equal(t, []string{"--chdir", ".", "--var=region=us"}, call.args)
	})

	t.Run("a changed variable file is rejected with the plan", func(t *testing.T) {
		require.NoError(t, os.WriteFile(varFile, []byte(`{"region":"changed"}`), 0o600))
		_, err := restoreConfigurationPlan(t.Context(), config, info)
		require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	})

	t.Run("an unreadable variable file fails the hook", func(t *testing.T) {
		require.NoError(t, os.WriteFile(varFile, []byte(`not json`), 0o600))
		_, err := run(t, info)
		require.ErrorIs(t, err, errUtils.ErrComponentHookFailed)
		require.NoError(t, os.Remove(varFile))
		_, err = run(t, info)
		require.ErrorIs(t, err, errUtils.ErrComponentHookFailed)
	})
}

func slicesContainsPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}
