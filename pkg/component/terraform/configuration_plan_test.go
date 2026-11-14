package terraform

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/policy/planfile"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_plan_runtime_test.go -package=$GOPACKAGE -mock_names=reviewedPlanRuntime=MockReviewedPlanRuntime,planIdentityRuntime=MockPlanIdentityRuntime,planShowRunner=MockPlanShowRunner

// reviewedPlanRuntime is a configuration runtime that prepares, binds saved plans, and resolves state.
type reviewedPlanRuntime interface {
	schema.ComponentConfigurationRuntime
	schema.ComponentConfigurationPlanRuntime
	schema.ComponentConfigurationStateRuntime
}

// planIdentityRuntime can bind plans but cannot resolve backend state.
type planIdentityRuntime interface {
	schema.ComponentConfigurationRuntime
	schema.ComponentConfigurationPlanRuntime
}

// planShowRunner runs the plan show command for plan policy evaluation.
type planShowRunner interface {
	process.Runner
}

// The generated mocks implement the contracts that the code under test asserts.
var (
	_ reviewedPlanRuntime = (*MockReviewedPlanRuntime)(nil)
	_ planIdentityRuntime = (*MockPlanIdentityRuntime)(nil)
	_ planShowRunner      = (*MockPlanShowRunner)(nil)
)

// planRuntimeFixture holds the responses that the generated plan runtime mock returns.
// Tests change its fields to simulate a different provider identity, backend, or failure.
type planRuntimeFixture struct {
	identity string
	state    *schema.ComponentConfigurationState
	err      error
}

// mock returns a generated plan runtime that answers PlanIdentity and ResolveState from the
// fixture at call time. Discover and Prepare have no expectations, so gomock rejects them.
func (p *planRuntimeFixture) mock(t *testing.T) *MockReviewedPlanRuntime {
	t.Helper()
	runtime := NewMockReviewedPlanRuntime(gomock.NewController(t))
	runtime.EXPECT().PlanIdentity(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *schema.ConfigAndStacksInfo, []schema.ComponentConfigurationSource) (string, error) {
			return p.identity, p.err
		},
	).AnyTimes()
	runtime.EXPECT().ResolveState(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
			return p.state, p.err
		},
	).AnyTimes()
	return runtime
}

func planFixture(t *testing.T) (*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, *componentExecContext, *planRuntimeFixture, string) {
	t.Helper()
	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: root, Components: schema.Components{Terraform: schema.Terraform{BasePath: "components"}}}
	info := &schema.ConfigAndStacksInfo{Stack: "live", ComponentFromArg: "network", FinalComponent: "network", SubCommand: "plan", ComponentSettingsSection: map[string]any{"configuration": true}}
	working := ConstructTerraformComponentWorkingDir(config, info)
	require.NoError(t, os.MkdirAll(working, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(working, "main.tf"), []byte("original module"), 0o600))
	binary := filepath.Join(root, "terraform.exe")
	require.NoError(t, os.WriteFile(binary, []byte("selected engine"), 0o700))
	info.Command = binary
	path := filepath.Join(working, "reviewed.planfile")
	require.NoError(t, os.WriteFile(path, []byte("reviewed plan"), 0o600))
	prepared := &schema.PreparedComponentConfiguration{Engine: "terraform", Source: root, Inputs: map[string]any{"value": "reviewed"}, BackendType: "local", Backend: map[string]any{"path": filepath.Join(root, "state")}, Workspace: "default"}
	runtime := &planRuntimeFixture{identity: "provider-source-options", state: &schema.ComponentConfigurationState{BackendType: prepared.BackendType, Backend: prepared.Backend, Workspace: prepared.Workspace}}
	config.ComponentConfigurations = runtime.mock(t)
	execCtx := &componentExecContext{componentPath: working, workingDir: working, preparedConfiguration: prepared}
	return config, info, execCtx, runtime, path
}

func TestConfigurationPlanRestoresReviewedInputs(t *testing.T) {
	config, info, execCtx, _, path := planFixture(t)
	err := recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"plan", "-out", filepath.Base(path)})
	require.NoError(t, err)
	stat, err := os.Stat(path + configurationPlanSuffix)
	require.NoError(t, err)
	// Windows reports 0666 for every writable file, so POSIX permission bits are only meaningful elsewhere.
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0), stat.Mode().Perm()&0o077)
	}
	info.SubCommand = "apply"
	info.UseTerraformPlan = true
	info.PlanFile = path
	prepared, err := restoreConfigurationPlan(t.Context(), config, info)
	require.NoError(t, err)
	require.Equal(t, execCtx.preparedConfiguration.Inputs, prepared.Inputs)
	require.True(t, info.UseTerraformPlan)
	require.Equal(t, path, info.PlanFile)
	require.FileExists(t, path)
}

func TestConfigurationPlanRejectsChangedBinding(t *testing.T) {
	for _, change := range []string{"plan", "engine", "provider", "workdir", "downloaded module", "backend", "workspace", "coordinates", "missing binding", "corrupt binding", "missing plan", "runtime error", "unsupported operation", "stored plan"} {
		t.Run(change, func(t *testing.T) {
			config, info, execCtx, runtime, path := planFixture(t)
			module := filepath.Join(execCtx.workingDir, ".terraform", "modules", "child", "script.sh")
			if change == "downloaded module" {
				require.NoError(t, os.MkdirAll(filepath.Dir(module), 0o700))
				require.NoError(t, os.WriteFile(module, []byte("reviewed"), 0o600))
			}
			require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"plan", "-out=" + path}))
			info.SubCommand = "apply"
			info.UseTerraformPlan = true
			info.PlanFile = path
			switch change {
			case "plan":
				require.NoError(t, os.WriteFile(path, []byte("unreviewed"), 0o600))
			case "engine":
				require.NoError(t, os.WriteFile(info.Command, []byte("changed executable"), 0o700))
			case "provider":
				runtime.identity = "other provider"
			case "workdir":
				require.NoError(t, os.WriteFile(filepath.Join(execCtx.workingDir, "main.tf"), []byte("modified"), 0o600))
			case "downloaded module":
				require.NoError(t, os.WriteFile(module, []byte("changed"), 0o600))
			case "backend":
				runtime.state.Backend = map[string]any{"path": "other"}
			case "workspace":
				runtime.state.Workspace = "other"
			case "coordinates":
				info.Stack = "other"
			case "missing binding":
				require.NoError(t, os.Remove(path+configurationPlanSuffix))
			case "corrupt binding":
				require.NoError(t, os.WriteFile(path+configurationPlanSuffix, []byte("not json"), 0o600))
			case "missing plan":
				require.NoError(t, os.Remove(path))
			case "runtime error":
				runtime.err = context.Canceled
			case "unsupported operation":
				info.SubCommand = "destroy"
			case "stored plan":
				info.StoredPlanFile = path
			}
			prepared, err := restoreConfigurationPlan(t.Context(), config, info)
			require.Error(t, err)
			require.Nil(t, prepared)
		})
	}
}

func TestConfigurationPlanRecordingGuards(t *testing.T) {
	config, info, execCtx, runtime, path := planFixture(t)
	for _, args := range [][]string{nil, {"-out"}, {"-out="}} {
		require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, args))
	}
	info.DryRun = true
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out", path}))
	info.DryRun = false
	info.SubCommand = "apply"
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out", path}))
	info.SubCommand = "plan"
	config.ComponentConfigurations = nil
	require.ErrorIs(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out", path}), errUtils.ErrPluginUnsupportedExtension)
	config.ComponentConfigurations = runtime.mock(t)
	runtime.err = context.Canceled
	require.ErrorIs(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out", path}), context.Canceled)
	runtime.err = nil
	require.Error(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out", path + "missing"}))
	info.Command = path + "missing"
	require.Error(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"-out", path}))
}

func TestConfigurationPlanPathsAndFileErrors(t *testing.T) {
	config, info, _, _, path := planFixture(t)
	require.Equal(t, ConstructTerraformComponentPlanfilePath(config, info), configurationPlanPath(config, info))
	info.PlanFile = filepath.Base(path)
	require.Equal(t, path, configurationPlanPath(config, info))
	info.PlanFile = path
	require.Equal(t, path, configurationPlanPath(config, info))
	err := writePlanBinding(filepath.Join(t.TempDir(), "absent", "binding"), nil)
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = fileDigest(t.TempDir())
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	_, err = fileDigest(filepath.Join(t.TempDir(), "absent"))
	require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = configurationWorkdirDigest(filepath.Join(t.TempDir(), "absent"), path)
	require.Error(t, err)
	require.True(t, usesConfigurationPlan(info))
	require.False(t, usesConfigurationPlan(&schema.ConfigAndStacksInfo{}))
}

func TestConfigurationWorkdirDigestIgnoresMutableFiles(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "reviewed.plan")
	first, err := configurationWorkdirDigest(root, plan)
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(root, ".terraform"), 0o700))
	for _, name := range []string{"terraform.tfstate", "reviewed.plan", "reviewed.plan.atmos.json"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("mutable"), 0o600))
	}
	second, err := configurationWorkdirDigest(root, plan)
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.tf"), []byte("source"), 0o600))
	third, err := configurationWorkdirDigest(root, plan)
	require.NoError(t, err)
	require.NotEqual(t, first, third)
	if err := os.Symlink(filepath.Join(root, "main.tf"), filepath.Join(root, "linked.tf")); err != nil {
		t.Skip("symlinks unavailable")
	}
	_, err = configurationWorkdirDigest(root, plan)
	require.True(t, errors.Is(err, errUtils.ErrPluginUnsupportedExtension))
}

func TestConfigurationWorkdirDigestIncludesDownloadedModules(t *testing.T) {
	root := t.TempDir()
	module := filepath.Join(root, ".terraform", "modules", "child", "script.sh")
	require.NoError(t, os.MkdirAll(filepath.Dir(module), 0o755))
	require.NoError(t, os.WriteFile(module, []byte("before"), 0o600))
	before, err := configurationWorkdirDigest(root, "")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(module, []byte("after"), 0o600))
	after, err := configurationWorkdirDigest(root, "")
	require.NoError(t, err)
	require.NotEqual(t, before, after)
}

func TestPlanOutputFlagsUseEffectiveArguments(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider []string
		args     []string
		conflict bool
		output   string
	}{
		{"double dash", nil, []string{"plan", "--out=new.plan"}, false, "new.plan"},
		{"provider output", []string{"--out=provider.plan"}, []string{"plan"}, false, "provider.plan"},
		{"provider and default conflict", []string{"--out=new.plan"}, []string{"plan", "-out=old.plan"}, true, ""},
		{"two separated flags conflict", nil, []string{"plan", "-out", "old.plan", "--out", "new.plan"}, true, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, info, execCtx, _, _ := planFixture(t)
			// A permissive CI exit mapping must not turn a preflight rejection into success.
			config.CI.Enabled = true
			config.Components.Terraform.CI.ExitCodes = map[int]bool{0: true, 1: true, 2: true}
			shell := &fakeShell{}
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.execute
			info.ComponentExecution = &schema.ComponentExecution{ExtraArguments: []schema.ComponentExtraArguments{{Commands: []string{"plan"}, Arguments: test.provider}}, BeforeHooks: []schema.ComponentHook{execHook("before", "plan")}}
			if test.output != "" {
				require.NoError(t, os.WriteFile(filepath.Join(execCtx.componentPath, test.output), []byte("plan"), 0o600))
			}
			err := executor.runMainCommandPhase(config, info, execCtx, test.args, execCtx.componentPath, false)
			if test.conflict {
				require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
				require.Empty(t, shell.calls, "neither hook nor Terraform should run")
			} else {
				require.NoError(t, err)
				require.FileExists(t, filepath.Join(execCtx.componentPath, test.output)+configurationPlanSuffix)
				require.Len(t, shell.calls, 2)
			}
		})
	}
}

func TestRecoveredPlanBindsEffectiveOutput(t *testing.T) {
	config, info, execCtx, _, _ := planFixture(t)
	config.Components.Terraform.Init.Reconfigure = schema.TerraformInitReconfigureAuto
	plan := filepath.Join(execCtx.componentPath, "recovered.plan")
	require.NoError(t, os.WriteFile(plan, []byte("reviewed"), 0o600))
	info.ComponentExecution = &schema.ComponentExecution{ExtraArguments: []schema.ComponentExtraArguments{{Commands: []string{"plan"}, Arguments: []string{"--out=" + plan}}}}
	attempts := 0
	shell := &fakeShell{run: func(call *shellCall) error {
		attempts++
		if attempts == 1 {
			return errors.New("Backend configuration changed")
		}
		return nil
	}}
	executor := testExecutor(t)
	executor.services.ExecuteShellCommand = shell.execute
	executor.executeTerraformInitForcedFn = func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string, string, bool, bool, ...ShellCommandOption) error {
		return nil
	}
	require.NoError(t, executor.runMainCommandPhase(config, info, execCtx, []string{"plan"}, execCtx.componentPath, false))
	require.Equal(t, 2, attempts)
	require.FileExists(t, plan+configurationPlanSuffix)
}

// TestDetailedExitCodePlanRecordsReviewedPlan binds a plan that reports changes with status 2,
// and leaves an ordinary failure unbound.
func TestDetailedExitCodePlanRecordsReviewedPlan(t *testing.T) {
	for _, test := range []struct {
		name   string
		extra  bool
		args   []string
		code   int
		record bool
	}{
		{name: "explicit detailed exit code with changes", args: []string{"plan", "-detailed-exitcode", "-out", "changes.plan"}, code: 2, record: true},
		{name: "provider detailed exit code", extra: true, args: []string{"plan", "-out", "changes.plan"}, code: 2, record: true},
		{name: "failure with detailed exit code", args: []string{"plan", "-detailed-exitcode", "-out", "changes.plan"}, code: 1},
		{name: "status 2 without detailed exit code", args: []string{"plan", "-out", "changes.plan"}, code: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, info, execCtx, _, _ := planFixture(t)
			if test.extra {
				info.ComponentExecution = &schema.ComponentExecution{ExtraArguments: []schema.ComponentExtraArguments{{Commands: []string{"plan"}, Arguments: []string{"-detailed-exitcode"}}}}
			}
			shell := &fakeShell{run: func(call *shellCall) error {
				if err := os.WriteFile(plannedOutputPath(execCtx.componentPath, call.args), []byte("changes"), 0o600); err != nil {
					return err
				}
				return errUtils.ExitCodeError{Code: test.code}
			}}
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.execute
			err := executor.runMainCommandPhase(config, info, execCtx, test.args, execCtx.componentPath, false)
			require.Equal(t, test.code, resolveExitCode(err), "the plan status is returned unchanged")
			binding := filepath.Join(execCtx.componentPath, "changes.plan") + configurationPlanSuffix
			if test.record {
				require.FileExists(t, binding)
				return
			}
			require.NoFileExists(t, binding)
		})
	}
}

// newPlanShowRunner returns a generated runner whose plan show prints an empty JSON plan.
func newPlanShowRunner(t *testing.T) process.Runner {
	t.Helper()
	runner := NewMockPlanShowRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, spec process.TaskSpec) process.Result {
			_, _ = io.WriteString(spec.Streams.Stdout, `{"format_version":"1.2","planned_values":{}}`)
			return process.Result{Started: true}
		},
	).AnyTimes()
	return runner
}

func TestPolicyPlanBindsPublishedOutput(t *testing.T) {
	config, info, execCtx, _, _ := planFixture(t)
	var engine policy.Engine
	require.NoError(t, engine.Register(policy.CELEngine, &policy.CEL{}))
	execCtx.planPolicy = &planfile.Guard{Engine: &engine, Show: newPlanShowRunner(t), Rules: []policy.Rule{{ID: "allow", On: policy.TerraformPlan, Engine: policy.CELEngine, Expression: "false", Effects: policy.Effects{Deny: true}}}}
	shell := &fakeShell{run: func(call *shellCall) error {
		path := plannedOutputPath(execCtx.componentPath, call.args)
		return os.WriteFile(path, []byte("reviewed"), 0o600)
	}}
	executor := testExecutor(t)
	executor.services.ExecuteShellCommand = shell.execute
	path := filepath.Join(execCtx.componentPath, "approved.plan")
	require.NoError(t, executor.runMainCommandPhase(config, info, execCtx, []string{"plan", "-out=" + path}, execCtx.componentPath, false))
	require.FileExists(t, path+configurationPlanSuffix)
}

// TestPlanOutputFlagsInEnvironmentAreRejected covers output flags that Terraform
// reads from TF_CLI_ARGS or TF_CLI_ARGS_plan, which Atmos cannot bind.
func TestPlanOutputFlagsInEnvironmentAreRejected(t *testing.T) {
	for _, test := range []struct {
		name      string
		inherited map[string]string
		component []string
		provider  map[string]string
		global    map[string]string
		sanitized []string
		commands  []string
		reject    bool
	}{
		{name: "inherited TF_CLI_ARGS_plan", inherited: map[string]string{"TF_CLI_ARGS_plan": "-out=env.plan"}, reject: true},
		{name: "inherited TF_CLI_ARGS", inherited: map[string]string{"TF_CLI_ARGS": "-lock=false --out env.plan"}, reject: true},
		{name: "component environment", component: []string{"TF_CLI_ARGS_plan=-out env.plan"}, reject: true},
		{name: "provider environment", provider: map[string]string{"TF_CLI_ARGS_plan": "--out=env.plan"}, commands: []string{"plan"}, reject: true},
		{name: "bare flag", component: []string{"TF_CLI_ARGS=-out"}, reject: true},
		{name: "other flags", inherited: map[string]string{"TF_CLI_ARGS_plan": "-lock=false -outfile-like=x"}, component: []string{"TF_CLI_ARGS=-no-color"}},
		{name: "apply only variable", component: []string{"TF_CLI_ARGS_apply=-out=x"}},
		{name: "provider environment for apply", provider: map[string]string{"TF_CLI_ARGS_plan": "-out=env.plan"}, commands: []string{"apply"}},
		{name: "global environment", global: map[string]string{"TF_CLI_ARGS_plan": "-out=env.plan"}, reject: true},
		{name: "sanitized environment omits inherited flag", inherited: map[string]string{"TF_CLI_ARGS_plan": "-out=env.plan"}, sanitized: []string{"PATH=/bin"}},
		{name: "sanitized environment carries flag", sanitized: []string{"TF_CLI_ARGS=-out=env.plan"}, reject: true},
		{name: "component overrides inherited flag", inherited: map[string]string{"TF_CLI_ARGS_plan": "-out=env.plan"}, component: []string{"TF_CLI_ARGS_plan=-lock=false"}},
		{name: "component overrides global flag", global: map[string]string{"TF_CLI_ARGS": "-out=env.plan"}, component: []string{"TF_CLI_ARGS="}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TF_CLI_ARGS", "")
			t.Setenv("TF_CLI_ARGS_plan", "")
			for name, value := range test.inherited {
				t.Setenv(name, value)
			}
			config, info, execCtx, _, path := planFixture(t)
			config.Env = test.global
			info.SanitizedEnv = test.sanitized
			info.ComponentEnvList = test.component
			if test.provider != nil {
				info.ComponentExecution = &schema.ComponentExecution{ExtraArguments: []schema.ComponentExtraArguments{{Commands: test.commands, Environment: test.provider}}, BeforeHooks: []schema.ComponentHook{execHook("before", "plan")}}
			}
			shell := &fakeShell{}
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.execute
			err := executor.runMainCommandPhase(config, info, execCtx, []string{"plan", "-out", filepath.Base(path)}, execCtx.componentPath, false)
			if test.reject {
				require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
				require.Empty(t, shell.calls, "neither hook nor Terraform should run")
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, shell.calls)
			require.FileExists(t, path+configurationPlanSuffix)
		})
	}
}

// TestEnvironmentPlanOutputIgnoresUnownedPlans keeps ordinary components unaffected.
func TestEnvironmentPlanOutputIgnoresUnownedPlans(t *testing.T) {
	t.Setenv("TF_CLI_ARGS_plan", "-out=env.plan")
	config, info, execCtx, _, _ := planFixture(t)
	require.ErrorIs(t, validateEnvironmentPlanOutput(config, info, execCtx), errUtils.ErrPlanVerificationFailed)
	info.DryRun = true
	require.NoError(t, validateEnvironmentPlanOutput(config, info, execCtx))
	info.DryRun = false
	info.SubCommand = "apply"
	require.NoError(t, validateEnvironmentPlanOutput(config, info, execCtx))
	info.SubCommand = "plan"
	execCtx.preparedConfiguration = nil
	require.NoError(t, validateEnvironmentPlanOutput(config, info, execCtx))
}

func TestConfigurationPlanRoundTripNumericBackend(t *testing.T) {
	config, info, execCtx, fixture, path := planFixture(t)
	execCtx.preparedConfiguration.Backend["port"] = 1234
	execCtx.preparedConfiguration.Backend["timeout"] = json.Number("1.0")
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"plan", "-out", filepath.Base(path)}))
	fixture.state.Backend["port"] = float64(1234)
	fixture.state.Backend["timeout"] = 1
	info.SubCommand, info.UseTerraformPlan, info.PlanFile = "apply", true, path
	_, err := restoreConfigurationPlan(t.Context(), config, info)
	require.NoError(t, err)
	require.False(t, equalBackendConfiguration(map[string]any{"value": json.Number("9007199254740993")}, map[string]any{"value": json.Number("9007199254740992")}))
}
