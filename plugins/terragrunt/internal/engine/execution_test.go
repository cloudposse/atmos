package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
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

// A saved plan is applied from its plan-time preparation, but Terragrunt evaluates the
// configuration again for apply. Hooks that depend on the command are recorded for apply.
func TestPlanPreparationRecordsApplyTimeExecution(t *testing.T) {
	body := `
  before_hook "plan_only" {
    commands = ["plan", "apply"]
    execute  = ["tool", "plan"]
    if       = get_terraform_command() == "plan"
  }
  after_hook "notify" {
    commands = ["apply"]
    execute  = ["tool", "notify"]
    if       = get_terraform_command() == "apply"
  }
  after_hook "always" {
    commands = ["apply"]
    execute  = ["tool", "always"]
  }
`
	planned, err := Resolve(t.Context(), executionUnit(t, body))
	require.NoError(t, err)
	require.Equal(t, []string{"plan_only"}, hookNames(planned.Execution.BeforeHooks))
	require.Equal(t, []string{"always"}, hookNames(planned.Execution.AfterHooks))
	require.NotNil(t, planned.Execution.Apply)
	require.Empty(t, planned.Execution.Apply.BeforeHooks)
	require.Equal(t, []string{"notify", "always"}, hookNames(planned.Execution.Apply.AfterHooks))
	require.Nil(t, planned.Execution.Apply.Apply)

	apply := executionUnit(t, body)
	apply.Command = "apply"
	applied, err := Resolve(t.Context(), apply)
	require.NoError(t, err)
	require.Nil(t, applied.Execution.Apply, "apply already evaluates as apply")
	require.Equal(t, planned.Execution.Apply.AfterHooks, applied.Execution.AfterHooks)
}

// Configuration that only the apply-time evaluation reads also shapes the reviewed plan, so the
// plan's watched sources include it.
func TestPlanPreparationWatchesApplyTimeReads(t *testing.T) {
	request := nativeUnit(t, `locals {
  hooks = read_terragrunt_config(get_terraform_command() == "apply" ? "apply-hooks.hcl" : "plan-hooks.hcl")
}`)
	root := filepath.Dir(request.Path)
	for _, name := range []string{"apply-hooks.hcl", "plan-hooks.hcl"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("locals {}\n"), 0o600))
	}
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	var names []string
	for _, file := range result.Files {
		names = append(names, filepath.Base(file))
	}
	require.Contains(t, names, "plan-hooks.hcl")
	require.Contains(t, names, "apply-hooks.hcl", "apply-time reads must be watched by the saved plan")
	require.Contains(t, names, "terragrunt.hcl")
}

func TestPlanPreparationRecordsThatApplyRunsNothing(t *testing.T) {
	result, err := Resolve(t.Context(), executionUnit(t, `
  before_hook "plan_only" {
    commands = ["plan", "apply"]
    execute  = ["tool", "plan"]
    if       = get_terraform_command() == "plan"
  }
`))
	require.NoError(t, err)
	require.Len(t, result.Execution.BeforeHooks, 1)
	require.NotNil(t, result.Execution.Apply, "an empty apply-time evaluation must not fall back to plan hooks")
	require.Empty(t, result.Execution.Apply.BeforeHooks)
}

func TestPlanPreparationOmitsApplyWhenCommandIsNotObserved(t *testing.T) {
	for name, body := range map[string]string{
		"no command function": `
  after_hook "always" {
    commands = ["apply"]
    execute  = ["tool", "always"]
  }
`,
		"same result for apply": `
  after_hook "always" {
    commands = ["apply"]
    execute  = ["tool", "always"]
    if       = length(get_terraform_command()) > 0
  }
`,
	} {
		t.Run(name, func(t *testing.T) {
			result, err := Resolve(t.Context(), executionUnit(t, body))
			require.NoError(t, err)
			require.Equal(t, []string{"always"}, hookNames(result.Execution.AfterHooks))
			require.Nil(t, result.Execution.Apply)
		})
	}
}

func TestObservesCommandAssumesUnreadableFilesDo(t *testing.T) {
	require.True(t, observesCommand(afero.NewMemMapFs(), []string{"missing.hcl"}))
}

func hookNames(hooks []model.Hook) []string {
	var names []string
	for i := range hooks {
		names = append(names, hooks[i].Name)
	}
	return names
}

func TestPlanRejectsArgumentDependentSavedHooks(t *testing.T) {
	request := executionUnit(t, `before_hook "args" {
 commands = ["apply"]
 execute = ["echo", join(",", get_terraform_cli_args())]
}`)
	_, err := Resolve(t.Context(), request)
	require.ErrorIs(t, err, ErrUnsupportedExecution)
}

func TestPreventDestroyChecksExpandedArguments(t *testing.T) {
	request := executionUnit(t, `extra_arguments "destroy" {
 commands = ["plan"]
 arguments = ["-destroy"]
}`)
	file, err := os.OpenFile(request.Path, os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = file.WriteString("\nprevent_destroy = true\n")
	require.NoError(t, err)
	require.NoError(t, file.Close())
	_, err = Resolve(t.Context(), request)
	require.ErrorIs(t, err, ErrPreventDestroy)
	request.Arguments = []string{"-destroy=false"}
	_, err = Resolve(t.Context(), request)
	require.NoError(t, err)
}

func TestPlanApplyMetadataReusesPlanApprovedMocks(t *testing.T) {
	request := executionUnit(t, `before_hook "command" {
 commands = ["plan", "apply"]
 execute = ["echo", get_terraform_command()]
}`)
	dependency := filepath.Join(request.BasePath, "dependency")
	require.NoError(t, os.MkdirAll(dependency, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dependency, "terragrunt.hcl"), []byte("inputs = {}"), 0o644))
	file, err := os.OpenFile(request.Path, os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = file.WriteString(`
dependency "upstream" {
 config_path = "./dependency"
 skip_outputs = true
 mock_outputs = { value = "planned" }
 mock_outputs_allowed_terraform_commands = ["plan"]
}
inputs = { value = dependency.upstream.outputs.value }
`)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "planned", result.Inputs["value"])
	require.NotNil(t, result.Execution.Apply)
}
