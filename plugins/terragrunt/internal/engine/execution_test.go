package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

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
			require.ErrorContains(t, err, "no command to run")
		})
	}
	require.NoError(t, validateHook("before_hook", &model.Hook{Name: "h", Commands: []string{"plan"}, Execute: []string{"tool"}}))
}

// fixtureEngine returns a request whose executable is a fake OpenTofu that answers `version`.
func fixtureEngine(t *testing.T, request model.Request) model.Request {
	t.Helper()
	request.Tofu = installFixtureExecutable(t, t.TempDir(), toolchainTofuName)
	request.Environment = map[string]string{toolchainChildEnv: "1"}
	return request
}

func TestTerraformBinaryAnchorsRelativePaths(t *testing.T) {
	unit := filepath.Join("repo", "live", "terragrunt.hcl")
	assert.Equal(t, "tofu", terraformBinary(" tofu ", unit), "a bare name is left to the toolchain")
	assert.Empty(t, terraformBinary("", unit))
	assert.Equal(t, filepath.Join("repo", "live", "bin", "tofu"), terraformBinary("./bin/tofu", unit))
	absolute := filepath.Join(t.TempDir(), "tofu")
	assert.Equal(t, absolute, terraformBinary(absolute, unit))
}

func TestResolveReturnsTerraformBinaryAndValidatesTheEngine(t *testing.T) {
	request := fixtureEngine(t, nativeUnit(t, `terraform_binary = "tofu"`))
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "tofu", result.TerraformBinary)

	// A unit without terraform_binary never probes the executable.
	plain := nativeUnit(t, "")
	result, err = Resolve(t.Context(), plain)
	require.NoError(t, err)
	require.Empty(t, result.TerraformBinary)

	// An executable that is not Terraform or OpenTofu is rejected with the setting named.
	wrong := nativeUnit(t, `terraform_binary = "tool"`)
	wrong.Tofu = installFixtureExecutable(t, t.TempDir(), toolchainToolName)
	wrong.Environment = map[string]string{toolchainChildEnv: "1"}
	_, err = Resolve(t.Context(), wrong)
	problem := asProblem(t, err)
	require.Equal(t, "terraform_binary", problem.Key)
	require.Contains(t, problem.Summary, "not a Terraform or OpenTofu executable")
}

func TestDiscoveryReadsTerraformBinaryWithoutRunningIt(t *testing.T) {
	request := nativeUnit(t, `terraform_binary = "./bin/tofu"`)
	request.Phase = "discover"
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(physicalPath(t, filepath.Dir(request.Path)), "bin", "tofu"), result.TerraformBinary)
}

func TestErrorsBlockIsReturnedInDeclarationOrder(t *testing.T) {
	request := nativeUnit(t, `errors {
  retry "network" {
    retryable_errors   = ["(?s).*timeout.*", "!ignore me"]
    max_attempts       = 4
    sleep_interval_sec = 2
  }
  retry "quota" {
    retryable_errors   = ["quota"]
    max_attempts       = 2
    sleep_interval_sec = 0
  }
  ignore "known" {
    ignorable_errors = ["already exists"]
    message          = "resource existed"
    signals = {
      note  = "kept"
      other = "also"
    }
  }
}`)
	request.Phase = "discover"
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, []model.RetryPolicy{
		{Name: "network", Patterns: []string{"(?s).*timeout.*", "!ignore me"}, MaxAttempts: 4, SleepIntervalSec: 2},
		{Name: "quota", Patterns: []string{"quota"}, MaxAttempts: 2},
	}, result.Errors.Retry)
	require.Len(t, result.Errors.Ignore, 1)
	ignore := result.Errors.Ignore[0]
	require.Equal(t, "known", ignore.Name)
	require.Equal(t, "resource existed", ignore.Message)
	require.Equal(t, "kept", ignore.Signals["note"])
	require.Equal(t, "also", ignore.Signals["other"])
}

func TestErrorsBlockRejectsInvalidPolicies(t *testing.T) {
	for name, test := range map[string]struct{ body, key, message string }{
		"no attempts": {`retry "x" {
    retryable_errors = ["a"]
    max_attempts = 0
    sleep_interval_sec = 1
  }`, `errors.retry "x"`, "max_attempts"},
		"negative sleep": {`retry "x" {
    retryable_errors = ["a"]
    max_attempts = 1
    sleep_interval_sec = -1
  }`, `errors.retry "x"`, "sleep_interval_sec"},
		"bad retry pattern": {`retry "x" {
    retryable_errors = ["("]
    max_attempts = 1
    sleep_interval_sec = 1
  }`, `errors.retry "x"`, "Invalid error pattern"},
		"bad ignore pattern": {`ignore "y" {
    ignorable_errors = ["!("]
  }`, `errors.ignore "y"`, "Invalid error pattern"},
	} {
		t.Run(name, func(t *testing.T) {
			request := nativeUnit(t, "errors {\n  "+test.body+"\n}")
			request.Phase = "discover"
			_, err := Resolve(t.Context(), request)
			problem := asProblem(t, err)
			require.Equal(t, test.key, problem.Key)
			require.Contains(t, problem.Summary+problem.Detail, test.message)
			require.True(t, problem.Safe)
			require.NotEmpty(t, problem.Hints)
		})
	}
}

func TestExcludeBlockOnlyAppliesWhileItsConditionHolds(t *testing.T) {
	enabled := nativeUnit(t, "exclude {\n  if = true\n  actions = [\"plan\", \"destroy\"]\n  exclude_dependencies = true\n  no_run = true\n}")
	enabled.Phase = "discover"
	result, err := Resolve(t.Context(), enabled)
	require.NoError(t, err)
	noRun := true
	require.Equal(t, &model.Exclude{Actions: []string{"plan", "destroy"}, ExcludeDependencies: true, NoRun: &noRun}, result.Exclude)

	disabled := nativeUnit(t, "exclude {\n  if = false\n  actions = [\"all\"]\n}")
	disabled.Phase = "discover"
	result, err = Resolve(t.Context(), disabled)
	require.NoError(t, err)
	require.Nil(t, result.Exclude)

	absent := nativeUnit(t, "")
	absent.Phase = "discover"
	result, err = Resolve(t.Context(), absent)
	require.NoError(t, err)
	require.Nil(t, result.Exclude)
}

func TestExcludeActionMatching(t *testing.T) {
	for _, test := range []struct {
		actions []string
		command string
		want    bool
	}{
		{[]string{"plan"}, "plan", true},
		{[]string{"PLAN"}, "plan", true},
		{[]string{"plan"}, "apply", false},
		{[]string{"all"}, "output", true},
		{[]string{"all_except_output"}, "output", false},
		{[]string{"all_except_output"}, "plan", true},
		{nil, "plan", false},
	} {
		assert.Equal(t, test.want, ExcludeListsAction(test.actions, test.command), "%v %s", test.actions, test.command)
	}
}

func TestPreventsRunFollowsNoRun(t *testing.T) {
	yes, no := true, false
	all := []string{"all"}
	for name, test := range map[string]struct {
		exclude *model.Exclude
		command string
		want    bool
	}{
		"no block":                   {nil, "plan", false},
		"unset, exact action":        {&model.Exclude{Actions: []string{"plan"}}, "plan", true},
		"unset, all does not apply":  {&model.Exclude{Actions: all}, "plan", false},
		"unset, other action":        {&model.Exclude{Actions: []string{"apply"}}, "plan", false},
		"true honors all":            {&model.Exclude{Actions: all, NoRun: &yes}, "plan", true},
		"true honors all_except":     {&model.Exclude{Actions: []string{"all_except_output"}, NoRun: &yes}, "output", false},
		"false never prevents":       {&model.Exclude{Actions: []string{"plan"}, NoRun: &no}, "plan", false},
		"true, command not listed":   {&model.Exclude{Actions: []string{"apply"}, NoRun: &yes}, "plan", false},
		"true, case insensitive set": {&model.Exclude{Actions: []string{"PLAN"}, NoRun: &yes}, "plan", true},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, PreventsRun(test.exclude, test.command))
		})
	}
}

const featureUnit = `
feature "enabled" {
  default = false
}
feature "size" {
  default = 3
}
feature "tier" {
  default = "basic"
}
inputs = {
  enabled = feature.enabled.value
  size    = feature.size.value
  tier    = feature.tier.value
}
`

func TestFeatureDefaultsAreRenderedAsStrings(t *testing.T) {
	result, err := Resolve(t.Context(), nativeUnit(t, featureUnit))
	require.NoError(t, err)
	require.Equal(t, map[string]string{"enabled": "false", "size": "3", "tier": "basic"}, result.FeatureFlags)
	require.Equal(t, false, result.Inputs["enabled"])
	require.Equal(t, "basic", result.Inputs["tier"])
}

func TestFeatureOverridesReplaceDefaults(t *testing.T) {
	request := nativeUnit(t, featureUnit)
	request.Features = map[string]string{"enabled": "true", "tier": "premium", "size": "8"}
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"enabled": "true", "size": "8", "tier": "premium"}, result.FeatureFlags)
	require.Equal(t, true, result.Inputs["enabled"], "a boolean feature is typed from its override")
	require.Equal(t, "premium", result.Inputs["tier"])
}

func TestFeatureOverrideOfWrongTypeNamesTheFeature(t *testing.T) {
	request := nativeUnit(t, featureUnit)
	request.Features = map[string]string{"enabled": "sometimes"}
	_, err := Resolve(t.Context(), request)
	require.Error(t, err)
	require.ErrorContains(t, err, "enabled")
}

func TestFeatureCanChooseADependencyOrSource(t *testing.T) {
	request := nativeUnit(t, `
feature "wide" {
  default = false
}
exclude {
  if      = feature.wide.value
  actions = ["plan"]
}
`)
	request.Phase = "discover"
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Nil(t, result.Exclude)

	request.Features = map[string]string{"wide": "true"}
	result, err = Resolve(t.Context(), request)
	require.NoError(t, err)
	require.NotNil(t, result.Exclude, "the override must reach the discovery evaluation too")
}

func TestParseFeatureEnvironment(t *testing.T) {
	parsed, err := ParseFeatureEnvironment(" a=1, b = text ,,c=")
	require.NoError(t, err)
	require.Equal(t, map[string]string{"a": "1", "b": "text", "c": ""}, parsed)

	for _, bad := range []string{"novalue", "=x"} {
		_, err = ParseFeatureEnvironment(bad)
		problem := asProblem(t, err)
		require.Equal(t, "TG_FEATURE", problem.Key)
	}
}

func TestFeatureOverrideWarningsNameUnusedKeys(t *testing.T) {
	inventory := []model.DiscoveredUnit{{Config: model.Resolution{FeatureFlags: map[string]string{"known": "x"}}}}
	assert.Nil(t, featureOverrideWarnings(nil, inventory))
	assert.Empty(t, featureOverrideWarnings(map[string]string{"known": "1"}, inventory))
	warnings := featureOverrideWarnings(map[string]string{"typo": "1", "known": "1", "also": "2"}, inventory)
	require.Len(t, warnings, 2)
	require.Equal(t, WarningFeatureOverrideUnused, warnings[0].Code)
	require.Contains(t, warnings[0].Message, "`also`")
	require.Contains(t, warnings[1].Message, "`typo`")
}

func TestLifecycleHooksRunInTheUnitDirectory(t *testing.T) {
	request := executionUnit(t, `
  after_hook "read" {
    commands = ["terragrunt-read-config", "read-config"]
    execute  = ["tool", "read"]
  }
  before_hook "fetch" {
    commands    = ["init-from-module"]
    execute     = ["tool", "fetch"]
    working_dir = "elsewhere"
  }
  error_hook "download_failed" {
    commands  = ["init-from-module"]
    execute   = ["tool", "report"]
    on_errors = [".*"]
  }
  after_hook "mixed" {
    commands = ["init-from-module", "apply"]
    execute  = ["tool", "mixed"]
  }
`)
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	unit := physicalPath(t, filepath.Dir(request.Path))

	require.Equal(t, "read", result.Execution.AfterHooks[0].Name)
	require.Equal(t, unit, result.Execution.AfterHooks[0].WorkingDirectory)
	require.Equal(t, "elsewhere", result.Execution.BeforeHooks[0].WorkingDirectory, "an explicit working_dir is kept")
	require.Equal(t, unit, result.Execution.ErrorHooks[0].WorkingDirectory)

	// A hook listing both stages is split so each keeps its own default directory.
	require.Len(t, result.Execution.AfterHooks, 3)
	require.Equal(t, []string{"init-from-module"}, result.Execution.AfterHooks[1].Commands)
	require.Equal(t, unit, result.Execution.AfterHooks[1].WorkingDirectory)
	require.Equal(t, []string{"apply"}, result.Execution.AfterHooks[2].Commands)
	require.Empty(t, result.Execution.AfterHooks[2].WorkingDirectory)
}

func TestReadConfigBeforeHookIsReportedAsNeverRunning(t *testing.T) {
	result, err := Resolve(t.Context(), executionUnit(t, `
  before_hook "early" {
    commands = ["terragrunt-read-config"]
    execute  = ["tool"]
  }
  before_hook "disabled" {
    commands = ["read-config"]
    execute  = ["tool"]
    if       = false
  }
`))
	require.NoError(t, err)
	require.Len(t, result.Warnings, 1)
	require.Equal(t, WarningReadConfigBeforeHook, result.Warnings[0].Code)
	require.Contains(t, result.Warnings[0].Message, `"early"`)
}

func TestTflintHookBecomesInitAndLintCommands(t *testing.T) {
	request := executionUnit(t, `
  extra_arguments "vars" {
    commands           = ["plan"]
    arguments          = ["-var=region=eu"]
    required_var_files = ["required.tfvars"]
    env_vars           = { TF_VAR_zone = "a" }
  }
  before_hook "lint" {
    commands = ["plan"]
    execute  = ["tflint", "--terragrunt-external-tflint", "--minimum-failure-severity=error"]
  }
`)
	request.Phase = ""
	root := physicalPath(t, filepath.Dir(request.Path))
	config := filepath.Join(root, ".tflint.hcl")
	require.NoError(t, os.WriteFile(config, []byte("plugin \"x\" {}\n"), 0o644))
	body, err := os.ReadFile(request.Path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(request.Path, append(body, []byte("inputs = { name = \"web\", count = 2 }\n")...), 0o644))
	request = fixtureEngine(t, request)

	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Len(t, result.Execution.BeforeHooks, 2)
	initHook, lintHook := result.Execution.BeforeHooks[0], result.Execution.BeforeHooks[1]
	require.Equal(t, []string{"tflint", "--init", "--config", config, "--chdir", "."}, initHook.Execute)
	require.Equal(t, []string{
		"tflint", "--config", config, "--chdir", ".",
		"--var=count=2", "--var=name=web",
		"--var=zone=a", "--var=region=eu", "--var-file=required.tfvars",
		"--minimum-failure-severity=error",
	}, lintHook.Execute)
	require.Equal(t, "lint", initHook.Name)
	require.Equal(t, []string{"plan"}, lintHook.Commands)
}

func TestTflintHookWithoutConfigurationNamesTheHook(t *testing.T) {
	root := t.TempDir()
	unit := filepath.Join(root, "a", "b")
	require.NoError(t, os.MkdirAll(unit, 0o755))
	// No .tflint.hcl exists in any parent below the temporary root, so the search must end with a problem.
	_, err := tflintConfig(nil, unit)
	if err == nil {
		t.Skip("a .tflint.hcl exists above the temporary directory")
	}
	_, err = expandTflint(&model.Hook{Name: "lint", Commands: []string{"plan"}, Execute: []string{"tflint"}}, &model.Execution{}, filepath.Join(unit, "terragrunt.hcl"), nil)
	problem := asProblem(t, err)
	require.Equal(t, model.CodeUnsupportedHook, problem.Code)
	require.Contains(t, problem.Summary, `"lint"`)
	require.NotEmpty(t, problem.Hints)
}

func TestTflintConfigFlagForms(t *testing.T) {
	dir := t.TempDir()
	absolute := filepath.Join(dir, "abs.hcl")
	for _, arguments := range [][]string{
		{"tflint", "--config", absolute},
		{"tflint", "-c", absolute},
		{"tflint", "--config=" + absolute},
		{"tflint", "-c=" + absolute},
	} {
		got, err := tflintConfig(arguments, dir)
		require.NoError(t, err)
		require.Equal(t, absolute, got)
	}
	got, err := tflintConfig([]string{"tflint", "--config", "rel.hcl"}, dir)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "rel.hcl"), got, "a relative config is anchored at the unit")
	require.Equal(t, []string{"--format", "json"}, withoutConfigFlag([]string{"--config", "x", "--format", "json", "-c=y", "--config=z"}))
}

func TestVariableValueRendering(t *testing.T) {
	text, err := variableValue("plain")
	require.NoError(t, err)
	require.Equal(t, "plain", text)
	text, err = variableValue(map[string]any{"a": []any{1, "${x}"}})
	require.NoError(t, err)
	require.Equal(t, `{"a":[1,"$${x}"]}`, text)
	_, err = variableValue(make(chan int))
	require.Error(t, err)
}

func TestCtyStringRendersEveryValueKind(t *testing.T) {
	assert.Equal(t, "text", ctyString(cty.StringVal("text")))
	assert.Equal(t, "true", ctyString(cty.True))
	assert.Equal(t, "2.5", ctyString(cty.NumberFloatVal(2.5)))
	assert.Equal(t, "7", ctyString(cty.NumberIntVal(7)))
	assert.Equal(t, `["a","b"]`, ctyString(cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")})))
	assert.Equal(t, "", ctyString(cty.NullVal(cty.String)))
	assert.Equal(t, "", ctyString(cty.UnknownVal(cty.String)))
	assert.Equal(t, "secret", ctyString(cty.StringVal("secret").Mark("sensitive")))
}

func TestSignalValuesConvertToPlainJSON(t *testing.T) {
	assert.Nil(t, mustSignals(t, nil))
	signals := mustSignals(t, map[string]cty.Value{
		"text":    cty.StringVal("x"),
		"number":  cty.NumberIntVal(3),
		"list":    cty.TupleVal([]cty.Value{cty.True, cty.StringVal("y")}),
		"nothing": cty.NullVal(cty.String),
		"unknown": cty.UnknownVal(cty.String),
	})
	assert.Equal(t, "x", signals["text"])
	assert.EqualValues(t, 3, signals["number"])
	assert.Equal(t, []any{true, "y"}, signals["list"])
	assert.Nil(t, signals["nothing"])
	assert.Nil(t, signals["unknown"])
}

func mustSignals(t *testing.T, signals map[string]cty.Value) map[string]any {
	t.Helper()
	result, err := signalValues(signals)
	require.NoError(t, err)
	return result
}

func TestFinalizeExecutionHandlesAbsentAndFailingHooks(t *testing.T) {
	require.NoError(t, finalizeExecution(nil, "terragrunt.hcl", nil))
	unit := filepath.Join(t.TempDir(), "terragrunt.hcl")
	for name, execution := range map[string]*model.Execution{
		"before": {BeforeHooks: []model.Hook{{Name: "lint", Commands: []string{"plan"}, Execute: []string{"tflint"}}}},
		"after":  {AfterHooks: []model.Hook{{Name: "lint", Commands: []string{"plan"}, Execute: []string{"tflint"}}}},
		"error":  {ErrorHooks: []model.Hook{{Name: "lint", Commands: []string{"plan"}, Execute: []string{"tflint"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := tflintConfig(nil, filepath.Dir(unit)); err == nil {
				t.Skip("a .tflint.hcl exists above the temporary directory")
			}
			err := finalizeExecution(execution, unit, nil)
			require.Equal(t, model.CodeUnsupportedHook, asProblem(t, err).Code)
		})
	}
	// Inputs that cannot be rendered fail the whole unit with the error, not a silent gap.
	config := filepath.Join(filepath.Dir(unit), ".tflint.hcl")
	require.NoError(t, os.WriteFile(config, nil, 0o644))
	err := finalizeExecution(&model.Execution{BeforeHooks: []model.Hook{{Name: "lint", Commands: []string{"plan"}, Execute: []string{"tflint"}}}}, unit, map[string]any{"bad": make(chan int)})
	require.Error(t, err)
}

func TestTflintVariablesFromExtraArguments(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "optional.tfvars")
	require.NoError(t, os.WriteFile(existing, nil, 0o644))
	hook := &model.Hook{Commands: []string{"plan"}}
	execution := &model.Execution{ExtraArguments: []model.ExtraArguments{
		{Commands: []string{"apply"}, Arguments: []string{"-var=ignored=1"}},
		{Commands: []string{"plan"}, Arguments: []string{"-var-file=a.tfvars", "-lock=false"}, OptionalVarFiles: []string{existing, "absent.tfvars"}, Environment: map[string]string{"OTHER": "x", "TF_VAR_b": "2"}},
	}}
	require.Equal(t, []string{"--var=b=2", "--var-file=a.tfvars", "--var-file=" + existing}, tflintExtraVariables(hook, execution))
}

func TestExcludeFromConfigWithoutBlock(t *testing.T) {
	assert.Nil(t, excludeFromConfig(nil))
	assert.Nil(t, excludeFromConfig(&config.ExcludeConfig{If: false}))
	assert.Equal(t, &model.Exclude{Actions: []string{"plan"}}, excludeFromConfig(&config.ExcludeConfig{If: true, Actions: []string{"plan"}}))
	assert.Nil(t, mustErrors(t, nil))
	assert.Nil(t, mustErrors(t, &config.ErrorsConfig{}))
}

func mustErrors(t *testing.T, block *config.ErrorsConfig) *model.Errors {
	t.Helper()
	result, err := errorsFromConfig(block, "terragrunt.hcl")
	require.NoError(t, err)
	return result
}

func TestFeatureValuesIgnoreUndeclaredAndNilFlags(t *testing.T) {
	assert.Nil(t, mustFeatures(t, nil, map[string]string{"x": "1"}))
	tier := cty.StringVal("basic")
	values := mustFeatures(t, []*config.FeatureFlag{nil, {Name: "tier", Default: &tier}, {Name: "nodefault"}}, nil)
	assert.Equal(t, map[string]string{"tier": "basic"}, values)
}

func mustFeatures(t *testing.T, flags []*config.FeatureFlag, overrides map[string]string) map[string]string {
	t.Helper()
	result, err := featureFlagValues(flags, overrides, "terragrunt.hcl")
	require.NoError(t, err)
	return result
}

func TestCheckFeatureOverridesLeavesParseErrorsToTheEvaluator(t *testing.T) {
	failing := func() (*config.TerragruntConfig, error) { return nil, os.ErrInvalid }
	require.NoError(t, checkFeatureOverrides("terragrunt.hcl", map[string]string{"a": "1"}, failing))
	require.NoError(t, checkFeatureOverrides("terragrunt.hcl", nil, failing))
	flag := cty.True
	declared := func() (*config.TerragruntConfig, error) {
		return &config.TerragruntConfig{FeatureFlags: []*config.FeatureFlag{{Name: "on", Default: &flag}}}, nil
	}
	require.NoError(t, checkFeatureOverrides("terragrunt.hcl", map[string]string{"on": "false"}, declared))
	require.Error(t, checkFeatureOverrides("terragrunt.hcl", map[string]string{"on": "maybe"}, declared))
}
