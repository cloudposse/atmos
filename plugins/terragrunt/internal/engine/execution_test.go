package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// executionUnit writes a unit whose terraform block carries body, so hooks and extra
// arguments live in the one block Terragrunt allows.
func executionUnit(t *testing.T, body string) model.Request {
	t.Helper()
	root := t.TempDir()
	module := filepath.Join(root, "module")
	require.NoError(t, os.MkdirAll(module, 0o755))
	source, err := json.Marshal(filepath.ToSlash(module))
	require.NoError(t, err)
	path := filepath.Join(root, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(path, []byte("terraform {\n  source = "+string(source)+"\n"+body+"\n}\n"), 0o644))
	return model.Request{Path: path, BasePath: root, Command: "plan", Tofu: "must-not-execute", Environment: map[string]string{}}
}

func TestExecutionIsAbsentWithoutHooksOrArguments(t *testing.T) {
	result, err := Resolve(t.Context(), executionUnit(t, ""))
	require.NoError(t, err)
	require.Nil(t, result.Execution)
}

func TestExecutionPreservesDeclarationOrderAndFields(t *testing.T) {
	result, err := Resolve(t.Context(), executionUnit(t, `
  extra_arguments "second_declared_first" {
    commands  = ["plan", "apply"]
    arguments = ["-lock-timeout=5m", "-compact-warnings"]
  }
  extra_arguments "vars" {
    commands           = ["plan"]
    required_var_files = ["required.tfvars", "required-b.tfvars"]
    optional_var_files = ["optional.tfvars"]
    env_vars           = { TF_LOG = "INFO", ZED = "z" }
  }
  before_hook "b2" {
    commands = ["plan"]
    execute  = ["tool", "one", "two"]
  }
  before_hook "b1" {
    commands       = ["plan", "apply"]
    execute        = ["tool", "three"]
    working_dir    = "sub"
    suppress_stdout = true
  }
  after_hook "a1" {
    commands     = ["plan"]
    execute      = ["tool", "after"]
    run_on_error = true
  }
  error_hook "e1" {
    commands  = ["apply"]
    execute   = ["tool", "error"]
    on_errors = ["timeout", ".*denied.*"]
  }
`))
	require.NoError(t, err)
	require.NotNil(t, result.Execution)

	require.Equal(t, []model.ExtraArguments{
		{Commands: []string{"plan", "apply"}, Arguments: []string{"-lock-timeout=5m", "-compact-warnings"}},
		{
			Commands:         []string{"plan"},
			RequiredVarFiles: []string{"required.tfvars", "required-b.tfvars"},
			OptionalVarFiles: []string{"optional.tfvars"},
			Environment:      map[string]string{"TF_LOG": "INFO", "ZED": "z"},
		},
	}, result.Execution.ExtraArguments)

	require.Equal(t, []model.Hook{
		{Name: "b2", Commands: []string{"plan"}, Execute: []string{"tool", "one", "two"}},
		{Name: "b1", Commands: []string{"plan", "apply"}, Execute: []string{"tool", "three"}, WorkingDirectory: "sub", SuppressStdout: true},
	}, result.Execution.BeforeHooks)
	require.Equal(t, []model.Hook{
		{Name: "a1", Commands: []string{"plan"}, Execute: []string{"tool", "after"}, RunOnError: true},
	}, result.Execution.AfterHooks)
	require.Equal(t, []model.Hook{
		{Name: "e1", Commands: []string{"apply"}, Execute: []string{"tool", "error"}, OnErrors: []string{"timeout", ".*denied.*"}},
	}, result.Execution.ErrorHooks)
}

func TestExecutionSkipsHooksWhoseConditionIsFalse(t *testing.T) {
	result, err := Resolve(t.Context(), executionUnit(t, `
  before_hook "off" {
    commands = ["plan"]
    execute  = ["tool", "off"]
    if       = false
  }
  before_hook "on" {
    commands = ["plan"]
    execute  = ["tool", "on"]
    if       = true
  }
`))
	require.NoError(t, err)
	require.Len(t, result.Execution.BeforeHooks, 1)
	require.Equal(t, "on", result.Execution.BeforeHooks[0].Name)
}

func TestExecutionRejectsUnimplementedHookSemantics(t *testing.T) {
	for _, test := range []struct{ name, body, message string }{
		{"read-config before", `before_hook "x" {
    commands = ["terragrunt-read-config"]
    execute  = ["tool"]
  }`, "terragrunt-read-config"},
		{"read-config alias after", `after_hook "x" {
    commands = ["plan", "read-config"]
    execute  = ["tool"]
  }`, "read-config"},
		{"init-from-module error", `error_hook "x" {
    commands  = ["init-from-module"]
    execute   = ["tool"]
    on_errors = [".*"]
  }`, "init-from-module"},
		{"tflint built in", `before_hook "lint" {
    commands = ["plan"]
    execute  = ["tflint", "--init"]
  }`, "tflint"},
		{"invalid on_errors", `error_hook "bad" {
    commands  = ["plan"]
    execute   = ["tool"]
    on_errors = ["("]
  }`, "on_errors"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Resolve(t.Context(), executionUnit(t, test.body))
			require.ErrorIs(t, err, ErrUnsupportedExecution)
			require.ErrorContains(t, err, test.message)
		})
	}
}

// Terragrunt itself refuses a hook with nothing to run, so the unit never reaches the host.
func TestExecutionRejectsEmptyExecute(t *testing.T) {
	_, err := Resolve(t.Context(), executionUnit(t, `before_hook "empty" {
    commands = ["plan"]
    execute  = []
  }`))
	require.ErrorContains(t, err, "execute")
}

func TestExecutionIsNotEvaluatedForBackendIdentity(t *testing.T) {
	request := executionUnit(t, `
  before_hook "x" {
    commands = ["terragrunt-read-config"]
    execute  = ["tool"]
  }`)
	request.Phase = "backend"
	_, err := Resolve(t.Context(), request)
	require.NoError(t, err)
}

func TestExecutionErrorHooksCarryWorkingDirectoryAndSuppression(t *testing.T) {
	result, err := Resolve(t.Context(), executionUnit(t, `
  error_hook "quiet" {
    commands        = ["apply"]
    execute         = ["tool", "report"]
    on_errors       = [".*"]
    working_dir     = "reports"
    suppress_stdout = true
  }
  error_hook "plain" {
    commands  = ["apply"]
    execute   = ["tool", "plain"]
    on_errors = [".*"]
  }
`))
	require.NoError(t, err)
	require.Equal(t, []model.Hook{
		{Name: "quiet", Commands: []string{"apply"}, Execute: []string{"tool", "report"}, OnErrors: []string{".*"}, WorkingDirectory: "reports", SuppressStdout: true},
		{Name: "plain", Commands: []string{"apply"}, Execute: []string{"tool", "plain"}, OnErrors: []string{".*"}},
	}, result.Execution.ErrorHooks)
}

func TestExecutionFromConfigWithoutTerraformBlock(t *testing.T) {
	result, err := executionFromConfig(nil)
	require.NoError(t, err)
	require.Nil(t, result, "a unit without a terraform block declares no hooks or arguments")
}

func TestValidateHookRequiresRunnableExecuteList(t *testing.T) {
	for name, execute := range map[string][]string{"missing": nil, "empty": {}, "blank executable": {""}} {
		t.Run(name, func(t *testing.T) {
			err := validateHook("before_hook", &model.Hook{Name: "h", Commands: []string{"plan"}, Execute: execute})
			require.ErrorIs(t, err, ErrUnsupportedExecution)
			require.ErrorContains(t, err, "non-empty execute list")
		})
	}
	require.NoError(t, validateHook("before_hook", &model.Hook{Name: "h", Commands: []string{"plan"}, Execute: []string{"tool"}}))
}
