package terraform

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
)

// shellCall is one invocation of the host's process facility.
type shellCall struct {
	command string
	args    []string
	dir     string
	env     []string
	dryRun  bool
	options component.ExecutionOptions
}

type hookContextKey struct{}

// fakeShell records every process the executor starts and lets a test script the outcome.
type fakeShell struct {
	mu    sync.Mutex
	calls []shellCall
	// run decides the result and may write to the option writers like a real process would.
	run func(call *shellCall) error
}

//nolint:gocritic // The signature is fixed by Services.ExecuteShellCommand.
func (f *fakeShell) execute(_ schema.AtmosConfiguration, command string, args []string, dir string, env []string, dryRun bool, _ string, opts ...component.ExecutionOption) error {
	call := shellCall{command: command, args: append([]string(nil), args...), dir: dir, env: append([]string(nil), env...), dryRun: dryRun}
	for _, opt := range opts {
		opt(&call.options)
	}
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
	if f.run == nil {
		return nil
	}
	return f.run(&call)
}

// names lists "<command> <first argument>" per call, which is enough to read the timeline.
func (f *fakeShell) names() []string {
	var names []string
	for i := range f.calls {
		call := &f.calls[i]
		name := filepath.Base(call.command)
		if len(call.args) > 0 {
			name += " " + call.args[0]
		}
		names = append(names, name)
	}
	return names
}

func hookExecutor(shell *fakeShell) *Executor {
	return NewExecutor(Services{OperationServices: nil, ExecuteShellCommand: shell.execute})
}

func hookInfo(subCommand string, execution *schema.ComponentExecution) *schema.ConfigAndStacksInfo {
	return &schema.ConfigAndStacksInfo{
		SubCommand:         subCommand,
		Command:            "terraform",
		ComponentEnvList:   []string{"BASE=1", "PATH=/toolchain/bin"},
		ComponentExecution: execution,
	}
}

func execHook(name string, commands ...string) schema.ComponentHook {
	return schema.ComponentHook{Name: name, Commands: commands, Execute: []string{"hook-" + name, "arg"}}
}

func TestExtraArgumentsPosition(t *testing.T) {
	for _, test := range []struct {
		name string
		info schema.ConfigAndStacksInfo
		args []string
		want int
	}{
		{"plain command", schema.ConfigAndStacksInfo{SubCommand: "plan"}, []string{"plan", "-out", "p"}, 1},
		{"state subcommand", schema.ConfigAndStacksInfo{SubCommand: "state", SubCommand2: "list"}, []string{"state", "list", "x"}, 2},
		{"second subcommand not in arguments", schema.ConfigAndStacksInfo{SubCommand: "state", SubCommand2: "list"}, []string{"state", "-x"}, 1},
		{"no arguments", schema.ConfigAndStacksInfo{SubCommand: "plan"}, nil, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, extraArgumentsPosition(&test.info, test.args))
		})
	}
}

func TestRunExecutionPhaseWithoutExecutionIsUnchanged(t *testing.T) {
	shell := &fakeShell{}
	e := hookExecutor(shell)
	info := hookInfo("plan", nil)
	called := 0
	phase := &executionPhase{command: "plan", args: []string{"plan", "-x"}, insertAt: 1, componentPath: t.TempDir()}
	err := e.runExecutionPhase(&schema.AtmosConfiguration{}, info, phase, nil, func(got *schema.ConfigAndStacksInfo, args []string, _ []component.ExecutionOption) error {
		called++
		require.Same(t, info, got)
		require.Equal(t, []string{"plan", "-x"}, args)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, called)
	require.Empty(t, shell.calls)
}

func TestMainCommandAppliesExtraArgumentsAndEnvironment(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "required.tfvars"), nil, 0o600))
	shell := &fakeShell{}
	e := hookExecutor(shell)
	info := hookInfo("plan", &schema.ComponentExecution{ExtraArguments: []schema.ComponentExtraArguments{
		{Commands: []string{"plan"}, Arguments: []string{"-lock-timeout=1m"}, RequiredVarFiles: []string{"required.tfvars"}, Environment: map[string]string{"TF_LOG": "INFO"}},
		{Commands: []string{"apply"}, Arguments: []string{"-only-for-apply"}, Environment: map[string]string{"ONLY_APPLY": "1"}},
	}})
	require.NoError(t, e.runMainTerraformCommand(&schema.AtmosConfiguration{}, info, []string{"plan", "-out", "p.tfplan"}, dir))

	require.Len(t, shell.calls, 1)
	call := shell.calls[0]
	require.Equal(t, []string{"plan", "-lock-timeout=1m", "-var-file=required.tfvars", "-out", "p.tfplan"}, call.args)
	require.Equal(t, dir, call.dir)
	require.Equal(t, []string{"BASE=1", "PATH=/toolchain/bin", "TF_LOG=INFO"}, call.env)
	require.Equal(t, []string{"BASE=1", "PATH=/toolchain/bin"}, info.ComponentEnvList, "phase environment must not leak into the invocation")
}

func TestMainCommandMissingRequiredVarFileFailsBeforeAnyProcess(t *testing.T) {
	shell := &fakeShell{}
	info := hookInfo("plan", &schema.ComponentExecution{
		ExtraArguments: []schema.ComponentExtraArguments{{Commands: []string{"plan"}, RequiredVarFiles: []string{"missing.tfvars"}}},
		BeforeHooks:    []schema.ComponentHook{execHook("before", "plan")},
	})
	err := hookExecutor(shell).runMainTerraformCommand(&schema.AtmosConfiguration{}, info, []string{"plan"}, t.TempDir())
	require.ErrorIs(t, err, errUtils.ErrComponentVarFileMissing)
	require.Empty(t, shell.calls)
}

func TestHooksRunWithTheInvocationEnvironmentAndDirectory(t *testing.T) {
	dir := t.TempDir()
	absolute := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))
	shell := &fakeShell{}
	e := hookExecutor(shell)
	ctx := context.WithValue(t.Context(), hookContextKey{}, "marker")
	info := hookInfo("plan", &schema.ComponentExecution{
		ExtraArguments: []schema.ComponentExtraArguments{{Commands: []string{"plan"}, Environment: map[string]string{"FROM_EXTRA": "1"}}},
		BeforeHooks: []schema.ComponentHook{
			execHook("default-dir", "plan"),
			{Name: "relative", Commands: []string{"plan"}, Execute: []string{"hook-relative"}, WorkingDirectory: "sub"},
			{Name: "absolute", Commands: []string{"plan"}, Execute: []string{"hook-absolute"}, WorkingDirectory: absolute},
		},
	})
	require.NoError(t, e.runMainTerraformCommand(&schema.AtmosConfiguration{}, info, []string{"plan"}, dir, component.WithProcessContext(ctx), component.WithEnvironment([]string{"IDENTITY=1"})))

	require.Equal(t, []string{"hook-default-dir arg", "hook-relative", "hook-absolute", "terraform plan"}, shell.names())
	for _, call := range shell.calls[:3] {
		require.False(t, call.dryRun)
		require.Equal(t, ctx, call.options.Ctx, "hooks must share the command's cancellation")
		require.Equal(t, []string{"IDENTITY=1"}, call.options.ProcessEnv, "hooks must share the identity's process environment")
		require.Subset(t, call.env, []string{"BASE=1", "PATH=/toolchain/bin", "FROM_EXTRA=1", "TG_CTX_TF_PATH=terraform", "TG_CTX_COMMAND=plan"})
	}
	require.Equal(t, dir, shell.calls[0].dir)
	require.Equal(t, filepath.Join(dir, "sub"), shell.calls[1].dir)
	require.Equal(t, absolute, shell.calls[2].dir)
	require.Contains(t, shell.calls[0].env, "TG_CTX_HOOK_NAME=default-dir")
	require.Contains(t, shell.calls[1].env, "TG_CTX_HOOK_NAME=relative")
	require.NotContains(t, shell.calls[3].env, "TG_CTX_HOOK_NAME=relative", "hook context variables belong to hooks only")
}

func TestHookOutputGoesToTheUIStreamNotTheDataStream(t *testing.T) {
	var data, ui strings.Builder
	streams := process.Streams{Stdout: &data, Stderr: &ui}
	shell := &fakeShell{run: func(call *shellCall) error {
		if call.command == "hook-loud" || call.command == "hook-quiet" {
			_, _ = io.WriteString(call.options.StdoutOverride, call.command+" says hi\n")
			_, _ = io.WriteString(call.options.StdoutCapture, call.command+" says hi\n")
		}
		return nil
	}}
	info := hookInfo("plan", &schema.ComponentExecution{BeforeHooks: []schema.ComponentHook{
		{Name: "loud", Commands: []string{"plan"}, Execute: []string{"hook-loud"}}, {Name: "quiet", Commands: []string{"plan"}, Execute: []string{"hook-quiet"}, SuppressStdout: true},
	}})
	var commandCapture strings.Builder
	require.NoError(t, hookExecutor(shell).runMainTerraformCommand(&schema.AtmosConfiguration{}, info, []string{"plan"}, t.TempDir(),
		component.WithProcessStreams(streams), component.WithStdoutCapture(&commandCapture)))

	require.Equal(t, "hook-loud says hi\n", ui.String(), "visible hook stdout must reach the UI channel")
	require.Empty(t, data.String(), "hook output must never reach the data channel")
	require.Empty(t, commandCapture.String(), "hook output must not enter the command's capture")
	require.Equal(t, io.Discard, shell.calls[1].options.StdoutOverride, "suppress_stdout discards the display copy")
}

func TestFailingBeforeHookAbortsTheCommandAndSurfacesItsExitCode(t *testing.T) {
	shell := &fakeShell{run: func(call *shellCall) error {
		if call.command == "hook-b1" {
			return errUtils.ExitCodeError{Code: 23}
		}
		return nil
	}}
	info := hookInfo("apply", &schema.ComponentExecution{
		BeforeHooks: []schema.ComponentHook{execHook("b1", "apply"), execHook("b2", "apply")},
		AfterHooks:  []schema.ComponentHook{execHook("a1", "apply")},
	})
	err := hookExecutor(shell).runMainTerraformCommand(&schema.AtmosConfiguration{}, info, []string{"apply"}, t.TempDir())
	require.ErrorIs(t, err, errUtils.ErrComponentHookFailed)
	require.Equal(t, 23, resolveExitCode(err))
	require.Equal(t, []string{"hook-b1 arg"}, shell.names(), "neither the command nor later hooks may run")
}

func TestAfterHooksFollowRunOnErrorWhenTheCommandFails(t *testing.T) {
	shell := &fakeShell{run: func(call *shellCall) error {
		if call.command == "terraform" {
			return errUtils.ExitCodeError{Code: 1}
		}
		return nil
	}}
	always := execHook("always", "plan")
	always.RunOnError = true
	info := hookInfo("plan", &schema.ComponentExecution{AfterHooks: []schema.ComponentHook{execHook("skipped", "plan"), always}})
	err := hookExecutor(shell).runMainTerraformCommand(&schema.AtmosConfiguration{}, info, []string{"plan"}, t.TempDir())
	require.Equal(t, 1, resolveExitCode(err))
	require.Equal(t, []string{"terraform plan", "hook-always arg"}, shell.names())
}

func TestErrorHookMatchesTheFailedCommandOutput(t *testing.T) {
	for _, test := range []struct {
		name     string
		patterns []string
		stderr   string
		want     []string
	}{
		{"matching output runs the hook", []string{"AccessDenied"}, "Error: AccessDenied", []string{"terraform apply", "hook-recover arg"}},
		{"non matching output does not", []string{"AccessDenied"}, "Error: other", []string{"terraform apply"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			shell := &fakeShell{run: func(call *shellCall) error {
				if call.command != "terraform" {
					return nil
				}
				_, _ = io.WriteString(call.options.StderrCapture, test.stderr)
				return errUtils.ExitCodeError{Code: 1}
			}}
			info := hookInfo("apply", &schema.ComponentExecution{ErrorHooks: []schema.ComponentHook{
				{Name: "recover", Commands: []string{"apply"}, Execute: []string{"hook-recover", "arg"}, OnErrors: test.patterns},
			}})
			var callerCapture strings.Builder
			err := hookExecutor(shell).runMainTerraformCommand(&schema.AtmosConfiguration{}, info, []string{"apply"}, t.TempDir(), component.WithStderrCapture(&callerCapture))
			require.Equal(t, 1, resolveExitCode(err))
			require.Equal(t, test.want, shell.names())
			require.Equal(t, test.stderr, callerCapture.String(), "the caller's own capture must keep receiving the command output")
		})
	}
}

func TestDryRunAppliesArgumentsButNeverRunsHooks(t *testing.T) {
	shell := &fakeShell{}
	info := hookInfo("plan", &schema.ComponentExecution{
		ExtraArguments: []schema.ComponentExtraArguments{{Commands: []string{"plan"}, Arguments: []string{"-lock=false"}}},
		BeforeHooks:    []schema.ComponentHook{execHook("b", "plan")},
		AfterHooks:     []schema.ComponentHook{execHook("a", "plan")},
		ErrorHooks:     []schema.ComponentHook{{Name: "e", Commands: []string{"plan"}, Execute: []string{"hook-e"}, OnErrors: []string{".*"}}},
	})
	info.DryRun = true
	require.NoError(t, hookExecutor(shell).runMainTerraformCommand(&schema.AtmosConfiguration{}, info, []string{"plan"}, t.TempDir()))
	require.Equal(t, []string{"terraform plan"}, shell.names())
	require.Equal(t, []string{"plan", "-lock=false"}, shell.calls[0].args)
	require.True(t, shell.calls[0].dryRun)
}

func TestCancellationStopsRemainingHooksAndTheCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	shell := &fakeShell{run: func(call *shellCall) error {
		if call.command == "hook-b1" {
			cancel()
			return context.Canceled
		}
		return nil
	}}
	always := execHook("always", "plan")
	always.RunOnError = true
	info := hookInfo("plan", &schema.ComponentExecution{
		BeforeHooks: []schema.ComponentHook{execHook("b1", "plan"), always},
		AfterHooks:  []schema.ComponentHook{always},
	})
	err := hookExecutor(shell).runMainTerraformCommand(&schema.AtmosConfiguration{}, info, []string{"plan"}, t.TempDir(), component.WithProcessContext(ctx))
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []string{"hook-b1 arg"}, shell.names())
}

func TestDestroyCommandNameMatchesApplyDestroy(t *testing.T) {
	shell := &fakeShell{}
	info := hookInfo("apply", &schema.ComponentExecution{
		BeforeHooks:    []schema.ComponentHook{execHook("for-apply", "apply"), execHook("for-destroy", "destroy")},
		ExtraArguments: []schema.ComponentExtraArguments{{Commands: []string{"destroy"}, Arguments: []string{"-parallelism=1"}}},
	})
	require.NoError(t, hookExecutor(shell).runMainTerraformCommand(&schema.AtmosConfiguration{}, info, []string{"apply", "-destroy", "-auto-approve"}, t.TempDir()))
	require.Equal(t, []string{"hook-for-destroy arg", "terraform apply"}, shell.names())
	require.Equal(t, []string{"apply", "-parallelism=1", "-destroy", "-auto-approve"}, shell.calls[1].args)
}

func TestSavedPlanApplyDropsVariableArguments(t *testing.T) {
	shell := &fakeShell{}
	info := hookInfo("apply", &schema.ComponentExecution{ExtraArguments: []schema.ComponentExtraArguments{
		{Commands: []string{"apply"}, Arguments: []string{"-var=a=b", "-parallelism=1"}, RequiredVarFiles: []string{"never-needed.tfvars"}},
	}})
	info.UseTerraformPlan = true
	require.NoError(t, hookExecutor(shell).runMainTerraformCommand(&schema.AtmosConfiguration{}, info, []string{"apply", "-auto-approve", "plan.tfplan"}, t.TempDir()))
	require.Equal(t, []string{"apply", "-parallelism=1", "-auto-approve", "plan.tfplan"}, shell.calls[0].args)
}

// Hooks and extra arguments are matched per phase. The implicit init is "init" even when the
// user asked for plan, so plan hooks must not fire for it, and init hooks must not fire for plan.
func TestImplicitInitMatchesInitNotTheRequestedCommand(t *testing.T) {
	shell := &fakeShell{}
	e := hookExecutor(shell)
	execution := &schema.ComponentExecution{
		ExtraArguments: []schema.ComponentExtraArguments{
			{Commands: []string{"init"}, Arguments: []string{"-input=false"}, Environment: map[string]string{"INIT_ONLY": "1"}},
			{Commands: []string{"plan"}, Arguments: []string{"-lock=false"}, Environment: map[string]string{"PLAN_ONLY": "1"}},
		},
		BeforeHooks: []schema.ComponentHook{execHook("init-before", "init"), execHook("plan-before", "plan")},
		AfterHooks:  []schema.ComponentHook{execHook("init-after", "init"), execHook("plan-after", "plan")},
	}
	info := hookInfo("plan", execution)
	dir := t.TempDir()

	_, err := e.runTerraformInitSubprocess(&schema.AtmosConfiguration{}, info, dir, []string{"init", "-reconfigure"})
	require.NoError(t, err)
	require.NoError(t, e.runMainTerraformCommand(&schema.AtmosConfiguration{}, info, []string{"plan", "-out", "p"}, dir))

	require.Equal(t, []string{
		"hook-init-before arg", "terraform init", "hook-init-after arg",
		"hook-plan-before arg", "terraform plan", "hook-plan-after arg",
	}, shell.names())
	initCall, planCall := shell.calls[1], shell.calls[4]
	require.Equal(t, []string{"init", "-input=false", "-reconfigure"}, initCall.args)
	require.Contains(t, initCall.env, "INIT_ONLY=1")
	require.NotContains(t, initCall.env, "PLAN_ONLY=1")
	require.Equal(t, []string{"plan", "-lock=false", "-out", "p"}, planCall.args)
	require.Contains(t, planCall.env, "PLAN_ONLY=1")
	require.NotContains(t, planCall.env, "INIT_ONLY=1")
	require.Contains(t, shell.calls[0].env, "TG_CTX_COMMAND=init")
	require.Contains(t, shell.calls[3].env, "TG_CTX_COMMAND=plan")
}

func TestHooksListedOnlyForOtherCommandsNeverFire(t *testing.T) {
	shell := &fakeShell{run: func(call *shellCall) error {
		if call.command == "terraform" {
			return errUtils.ExitCodeError{Code: 1}
		}
		return nil
	}}
	e := hookExecutor(shell)
	always := execHook("always", "apply")
	always.RunOnError = true
	info := hookInfo("plan", &schema.ComponentExecution{
		ExtraArguments: []schema.ComponentExtraArguments{{Commands: []string{"apply"}, Arguments: []string{"-never"}}},
		BeforeHooks:    []schema.ComponentHook{execHook("b", "apply")},
		AfterHooks:     []schema.ComponentHook{always},
		ErrorHooks:     []schema.ComponentHook{{Name: "e", Commands: []string{"apply"}, Execute: []string{"hook-e"}, OnErrors: []string{".*"}}},
	})
	dir := t.TempDir()
	_, initErr := e.runTerraformInitSubprocess(&schema.AtmosConfiguration{}, info, dir, []string{"init"})
	require.Error(t, initErr)
	require.Error(t, e.runMainTerraformCommand(&schema.AtmosConfiguration{}, info, []string{"plan"}, dir))
	require.Equal(t, []string{"terraform init", "terraform plan"}, shell.names())
	require.Equal(t, []string{"init"}, shell.calls[0].args)
	require.Equal(t, []string{"plan"}, shell.calls[1].args)
}

func TestWorkspaceSelectionNeverRunsHooksOrExtraArguments(t *testing.T) {
	shell := &fakeShell{run: func(*shellCall) error { return nil }}
	e := hookExecutor(shell)
	info := hookInfo("plan", &schema.ComponentExecution{
		ExtraArguments: []schema.ComponentExtraArguments{{Commands: []string{"plan", "init", "workspace"}, Arguments: []string{"-never"}, Environment: map[string]string{"NEVER": "1"}}},
		BeforeHooks:    []schema.ComponentHook{execHook("b", "plan", "init", "workspace")},
		AfterHooks:     []schema.ComponentHook{execHook("a", "plan", "init", "workspace")},
	})
	info.TerraformWorkspace = "dev"
	require.NoError(t, e.runWorkspaceSetup(&schema.AtmosConfiguration{}, info, t.TempDir()))
	require.Equal(t, []string{"terraform workspace"}, shell.names())
	require.Equal(t, []string{"workspace", "select", "-or-create", "dev"}, shell.calls[0].args)
	require.NotContains(t, shell.calls[0].env, "NEVER=1")
}

func TestUnknownHookFailureIsNotMistakenForSuccess(t *testing.T) {
	shell := &fakeShell{run: func(call *shellCall) error {
		if call.command == "hook-b" {
			return errors.New("executable file not found")
		}
		return nil
	}}
	info := hookInfo("plan", &schema.ComponentExecution{BeforeHooks: []schema.ComponentHook{{Name: "b", Commands: []string{"plan"}, Execute: []string{"hook-b"}}}})
	err := hookExecutor(shell).runMainTerraformCommand(&schema.AtmosConfiguration{}, info, []string{"plan"}, t.TempDir())
	require.ErrorIs(t, err, errUtils.ErrComponentHookFailed)
	require.ErrorContains(t, err, "executable file not found")
	require.Equal(t, []string{"hook-b"}, shell.names())
}

// fakeConfigurationRuntime is a provider session that returns a fixed preparation.
type fakeConfigurationRuntime struct {
	prepared *schema.PreparedComponentConfiguration
}

func (fakeConfigurationRuntime) Discover(context.Context) (map[string]map[string]any, error) {
	return nil, nil
}

func (f fakeConfigurationRuntime) Prepare(context.Context, *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, error) {
	return f.prepared, nil
}

func TestPrepareConfigurationHandsExecutionToThePipeline(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	execution := &schema.ComponentExecution{BeforeHooks: []schema.ComponentHook{execHook("b", "plan")}}
	config := &schema.AtmosConfiguration{ComponentConfigurations: fakeConfigurationRuntime{prepared: &schema.PreparedComponentConfiguration{
		Source: "./module", BackendType: "local", Backend: map[string]any{}, Workspace: "default", Execution: execution,
	}}}
	info := &schema.ConfigAndStacksInfo{
		SubCommand: "plan", Command: executable, ComponentSection: map[string]any{},
		ComponentSettingsSection: map[string]any{"configuration": true},
	}
	prepared, err := prepareConfiguration(t.Context(), config, info)
	require.NoError(t, err)
	require.NotNil(t, prepared)
	require.Equal(t, execution, info.ComponentExecution)
}

// A reviewed plan carries its hooks and extra arguments in the private companion, and the
// companion decoder rejects unknown fields, so the new data must survive that exact decode.
func TestSavedPlanBindingKeepsExecution(t *testing.T) {
	plan := configurationPlan{Version: 1, Prepared: &schema.PreparedComponentConfiguration{
		Execution: &schema.ComponentExecution{
			ExtraArguments: []schema.ComponentExtraArguments{{Commands: []string{"apply"}, Environment: map[string]string{"K": "v"}}},
			ErrorHooks:     []schema.ComponentHook{{Name: "e", Commands: []string{"apply"}, Execute: []string{"tool"}, OnErrors: []string{"x"}}},
		},
	}}
	data, err := json.Marshal(plan)
	require.NoError(t, err)
	var decoded configurationPlan
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	require.NoError(t, decoder.Decode(&decoded))
	require.Equal(t, plan.Prepared.Execution, decoded.Prepared.Execution)
}

// exitCodeError mimics the error a Terraform subprocess returns with a given status.
func exitCodeError(code int) error { return errUtils.ExitCodeError{Code: code} }

func detailedExitCodeInfo() *schema.ConfigAndStacksInfo {
	return hookInfo("plan", &schema.ComponentExecution{
		AfterHooks: []schema.ComponentHook{execHook("after", "plan")},
		ErrorHooks: []schema.ComponentHook{{Name: "alert", Commands: []string{"plan"}, Execute: []string{"hook-alert"}, OnErrors: []string{".*"}}},
	})
}

// Exit status 2 of plan -detailed-exitcode means "changes present": hooks must see success,
// and the caller must still receive status 2.
func TestDetailedExitCodeChangesAreNotAHookFailure(t *testing.T) {
	shell := &fakeShell{run: func(call *shellCall) error {
		if call.command == "terraform" {
			return exitCodeError(2)
		}
		return nil
	}}
	err := hookExecutor(shell).runMainTerraformCommand(&schema.AtmosConfiguration{}, detailedExitCodeInfo(), []string{"plan", "-detailed-exitcode"}, t.TempDir())

	require.Error(t, err)
	require.Equal(t, 2, errUtils.GetExitCode(err), "the status must still reach the caller")
	require.Equal(t, []string{"terraform plan", "hook-after arg"}, shell.names(), "the after hook runs and the error hook does not")
}

func TestDetailedExitCodeIsOnlyToleratedWhereTerraformDefinesIt(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		code int
	}{
		{"status 2 without the flag is a failure", []string{"plan"}, 2},
		{"status 1 with the flag is a failure", []string{"plan", "-detailed-exitcode"}, 1},
		{"double dash flag", []string{"plan", "--detailed-exitcode"}, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			shell := &fakeShell{run: func(call *shellCall) error {
				if call.command == "terraform" {
					return exitCodeError(test.code)
				}
				return nil
			}}
			err := hookExecutor(shell).runMainTerraformCommand(&schema.AtmosConfiguration{}, detailedExitCodeInfo(), test.args, t.TempDir())
			require.Equal(t, test.code, errUtils.GetExitCode(err))
			tolerated := test.args[len(test.args)-1] == "--detailed-exitcode" || (test.code == 2 && len(test.args) > 1)
			if tolerated {
				require.Equal(t, []string{"terraform plan", "hook-after arg"}, shell.names())
				return
			}
			require.Equal(t, []string{"terraform plan", "hook-alert"}, shell.names(), "a real failure runs the error hook and skips the after hook")
		})
	}
}

func TestHookWorkingDirectoryErrorsNameTheDirectory(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(t.TempDir(), "absent")
	for name, test := range map[string]struct {
		directory string
		contains  []string
	}{
		"absolute": {missing, []string{missing}},
		"relative": {"scripts", []string{filepath.Join(dir, "scripts"), "get_terragrunt_dir()"}},
	} {
		t.Run(name, func(t *testing.T) {
			shell := &fakeShell{}
			info := hookInfo("plan", &schema.ComponentExecution{BeforeHooks: []schema.ComponentHook{
				{Name: "bad", Commands: []string{"plan"}, Execute: []string{"hook-bad"}, WorkingDirectory: test.directory},
			}})
			err := hookExecutor(shell).runMainTerraformCommand(&schema.AtmosConfiguration{}, info, []string{"plan"}, dir)
			require.ErrorIs(t, err, errUtils.ErrComponentHookWorkingDirectory)
			require.ErrorIs(t, err, errUtils.ErrComponentHookFailed)
			for _, want := range test.contains {
				require.Contains(t, err.Error(), want)
			}
			require.Empty(t, shell.calls, "the program must not be started in a missing directory")
		})
	}
}

func TestHookUnitLabel(t *testing.T) {
	require.Empty(t, hookUnitLabel(&schema.ConfigAndStacksInfo{}))
	require.Equal(t, " (network in live/dev)", hookUnitLabel(&schema.ConfigAndStacksInfo{ComponentFromArg: "network", Stack: "live/dev"}))
}

// A hook listing "state" matches every state subcommand, as in Terragrunt.
func TestHooksMatchTheTerraformCommandNotItsSubcommand(t *testing.T) {
	shell := &fakeShell{}
	info := hookInfo("state list", &schema.ComponentExecution{BeforeHooks: []schema.ComponentHook{execHook("state-hook", "state")}})
	require.NoError(t, hookExecutor(shell).runMainTerraformCommand(&schema.AtmosConfiguration{}, info, []string{"state", "list"}, t.TempDir()))
	require.Equal(t, []string{"hook-state-hook arg", "terraform state"}, shell.names())
}
