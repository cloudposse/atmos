package terraform

import (
	"errors"
	"fmt"
	"os"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/policy/planfile"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/require"
)

func TestDetailedExitCodePlanBinding(t *testing.T) {
	for _, test := range []struct {
		name                                                    string
		hooks, recoverInit, hookFailure, bindingFailure, noFlag bool
		code                                                    int
	}{
		{name: "plain plan", code: 2},
		{name: "hooks complete", hooks: true, code: 2},
		{name: "init recovery", hooks: true, recoverInit: true, code: 2},
		{name: "after hook fails with status two", hooks: true, hookFailure: true, code: 2},
		{name: "binding fails", hooks: true, bindingFailure: true, code: 2},
		{name: "plan fails", hooks: true, code: 1},
		{name: "status two without flag", hooks: true, noFlag: true, code: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, info, execCtx, runtime, path := planFixture(t)
			config.Components.Terraform.Init.Reconfigure = schema.TerraformInitReconfigureAuto
			args := []string{"plan", "-out=" + path}
			if !test.noFlag {
				args = append(args, "-detailed-exitcode")
			}
			if test.hooks {
				info.ComponentExecution = &schema.ComponentExecution{AfterHooks: []schema.ComponentHook{execHook("after", "plan")}}
			}
			bindingErr := errors.New("binding identity unavailable")
			if test.bindingFailure {
				runtime.err = bindingErr
			}
			attempts, initCalls, hookCalls := 0, 0, 0
			shell := &fakeShell{run: func(call *shellCall) error {
				if call.command == "hook-after" {
					hookCalls++
					if test.hookFailure {
						return exitCodeError(2)
					}
					return nil
				}
				attempts++
				if test.recoverInit && attempts == 1 {
					return errors.New("Backend configuration changed")
				}
				return exitCodeError(test.code)
			}}
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.execute
			executor.executeTerraformInitForcedFn = func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string, string, bool, bool, ...ShellCommandOption) error {
				initCalls++
				return nil
			}
			err := executor.runMainCommandPhase(config, info, execCtx, args, execCtx.componentPath, false)
			require.Error(t, err)
			if test.bindingFailure {
				require.ErrorIs(t, err, bindingErr)
			} else {
				require.Equal(t, test.code, errUtils.GetExitCode(err))
			}
			if test.recoverInit {
				require.Equal(t, 2, attempts)
				require.Equal(t, 1, initCalls)
			} else {
				require.Equal(t, 1, attempts)
				require.Zero(t, initCalls)
			}
			if test.hooks && test.code == 2 && !test.noFlag {
				require.Equal(t, 1, hookCalls)
			} else {
				require.Zero(t, hookCalls)
			}
			if test.hookFailure || test.bindingFailure || test.noFlag || test.code != 2 {
				require.NoFileExists(t, path+configurationPlanSuffix)
				return
			}
			require.FileExists(t, path+configurationPlanSuffix)
			info.SubCommand, info.UseTerraformPlan, info.PlanFile = "apply", true, path
			_, err = restoreConfigurationPlan(t.Context(), config, info)
			require.NoError(t, err, "the status-two plan must be usable by --from-plan")
		})
	}
}

func TestDetailedExitCodeBindingUsesSubprocessEnvironment(t *testing.T) {
	t.Setenv("TF_CLI_ARGS", "")
	for _, test := range []struct {
		name, ambient            string
		processEnv, componentEnv []string
		success                  bool
	}{
		{name: "inherit ambient", ambient: "-detailed-exitcode", success: true},
		{name: "replacement enables", processEnv: []string{"TF_CLI_ARGS_plan=-detailed-exitcode"}, success: true},
		{name: "empty replacement excludes ambient", ambient: "-detailed-exitcode", processEnv: []string{}},
		{name: "replacement excludes ambient", ambient: "-detailed-exitcode", processEnv: []string{"OTHER=value"}},
		{name: "component enables", processEnv: []string{}, componentEnv: []string{"TF_CLI_ARGS_plan=-detailed-exitcode"}, success: true},
		{name: "component disables", processEnv: []string{"TF_CLI_ARGS_plan=-detailed-exitcode"}, componentEnv: []string{"TF_CLI_ARGS_plan="}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TF_CLI_ARGS_plan", test.ambient)
			config, info, execCtx, _, path := planFixture(t)
			info.ComponentExecution = detailedExitCodeInfo().ComponentExecution
			info.ComponentEnvList = test.componentEnv
			shell := &fakeShell{run: func(call *shellCall) error {
				if call.command == info.Command {
					return exitCodeError(2)
				}
				return nil
			}}
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.execute
			err := executor.runMainCommandPhase(config, info, execCtx, []string{"plan", "-out=" + path}, execCtx.componentPath, false, component.WithEnvironment(test.processEnv))
			require.Equal(t, 2, errUtils.GetExitCode(err))
			require.Len(t, shell.calls, 2)
			if test.success {
				require.Equal(t, "hook-after", shell.calls[1].command)
				require.FileExists(t, path+configurationPlanSuffix)
			} else {
				require.Equal(t, "hook-alert", shell.calls[1].command)
				require.NoFileExists(t, path+configurationPlanSuffix)
			}
		})
	}
}

func TestDetailedExitCodePolicyPlanBinding(t *testing.T) {
	for _, deny := range []bool{false, true} {
		t.Run(fmt.Sprintf("deny=%t", deny), func(t *testing.T) {
			config, info, execCtx, _, path := planFixture(t)
			var engine policy.Engine
			require.NoError(t, engine.Register(policy.CELEngine, &policy.CEL{}))
			execCtx.planPolicy = &planfile.Guard{Engine: &engine, Show: planShowRunner{}, Rules: []policy.Rule{{ID: "guard", On: policy.TerraformPlan, Engine: policy.CELEngine, Expression: fmt.Sprint(deny), Effects: policy.Effects{Deny: true}}}}
			require.NoError(t, os.Remove(path))
			shell := &fakeShell{run: func(call *shellCall) error {
				require.NoError(t, os.WriteFile(plannedOutputPath(execCtx.componentPath, call.args), []byte("reviewed"), 0o600))
				return exitCodeError(2)
			}}
			executor := testExecutor(t)
			executor.services.ExecuteShellCommand = shell.execute
			err := executor.runMainCommandPhase(config, info, execCtx, []string{"plan", "-out=" + path, "-detailed-exitcode"}, execCtx.componentPath, false)
			if deny {
				require.Error(t, err)
				require.NotEqual(t, 2, errUtils.GetExitCode(err))
				require.NoFileExists(t, path)
				require.NoFileExists(t, path+configurationPlanSuffix)
			} else {
				require.Equal(t, 2, errUtils.GetExitCode(err))
				require.FileExists(t, path+configurationPlanSuffix)
			}
		})
	}
}
