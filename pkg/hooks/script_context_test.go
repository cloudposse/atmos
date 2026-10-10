package hooks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStarlarkHookLifecycleContext(t *testing.T) {
	for _, tc := range []struct {
		name    string
		event   HookEvent
		outcome Outcome
		expect  string
	}{
		{"before", BeforeTerraformApply, Outcome{Status: RunSuccess}, `ctx.operation.status == None and ctx.operation.exit_code == None and ctx.operation.error == None`},
		{"success", AfterTerraformDeploy, Outcome{Status: RunSuccess}, `ctx.operation.status == "success" and ctx.operation.exit_code == 0 and ctx.operation.error == None`},
		{"failure", AfterTerraformApply, Outcome{Status: RunFailure, ExitCode: 7, Err: errors.New("deployment failed")}, `ctx.operation.status == "failure" and ctx.operation.exit_code == 7 and ctx.operation.error == "deployment failed"`},
		{"unknown", AfterTerraformApply, Outcome{}, `ctx.operation.status == None and ctx.operation.exit_code == None`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hook := &Hook{Kind: stepKindName, Type: "script", OnFailure: OnFailureFail, With: map[string]any{
				"interpreter": "starlark", "output": "none", "script": `
def verify():
    if not (` + tc.expect + `):
        fail("incorrect operation result")
    if ctx.operation.command != "terraform apply" or ctx.hook.name != "verify":
        fail("incorrect lifecycle identity")
    if ctx.operation.stdout != None or ctx.operation.stderr != None:
        fail("hook writers are not parent process captures")
verify()
`,
			}}
			ctx := stepExecContext(hook)
			ctx.Event, ctx.Outcome, ctx.HookName = tc.event, tc.outcome, "verify"
			// Hook subprocess state must never replace the parent outcome.
			ctx.ExitCode, ctx.CommandError = 99, errors.New("hook subprocess error")
			_, err := stepEngine{}.Run(ctx)
			require.NoError(t, err)
		})
	}
}

func TestStarlarkHookSettingsAndParallelFunctions(t *testing.T) {
	dir := t.TempDir()
	// Hook steps default to the component directory, which must exist.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "shared"), 0o755))
	settings := map[string]any{"post_apply": map[string]any{"enabled": true, "services": []any{"api", "worker"}}}
	ctx := stepExecContext(&Hook{Kind: stepKindName, Type: "script", OnFailure: OnFailureFail, With: map[string]any{
		"interpreter": "starlark", "output": "none", "env": map[string]string{"EXPECTED_PATH": filepath.Join(dir, "shared")},
		"script": `
def branch(name):
    return [name, ctx.component.vars["region"], ctx.operation.exit_code]
def verify():
    c = ctx.component
    if c.name != "api" or c.stack != "dev" or c.type != "terraform" or c.implementation != "shared":
        fail("component binding lost")
    if c.path != env["EXPECTED_PATH"] or c.metadata["component"] != "shared":
        fail("component path or metadata lost")
    policy = c.settings["post_apply"]
    if not policy["enabled"]:
        fail("settings missing")
    results = steps.parallel(tasks=[steps.task(name=n, function=branch, args=[n]) for n in policy["services"]])
    if results != [["api", "us-east-1", 0], ["worker", "us-east-1", 0]]:
        fail("context missing from parallel branches")
verify()
`,
	}})
	ctx.AtmosConfig = &schema.AtmosConfiguration{TerraformDirAbsolutePath: dir}
	ctx.Info = &schema.ConfigAndStacksInfo{
		ComponentFromArg: "api", Stack: "dev", ComponentType: "terraform", FinalComponent: "shared",
		ComponentSection:         map[string]any{"metadata": map[string]any{"component": "shared"}},
		ComponentSettingsSection: settings, ComponentVarsSection: map[string]any{"region": "us-east-1"},
	}
	ctx.Event, ctx.Outcome = AfterTerraformApply, Outcome{Status: RunSuccess}
	_, err := stepEngine{}.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, true, settings["post_apply"].(map[string]any)["enabled"])

	ctx.Hook.With = map[string]any{"interpreter": "starlark", "output": "none", "script": `ctx.component.settings["post_apply"]["enabled"] = False`}
	_, err = stepEngine{}.Run(ctx)
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.ErrorContains(t, err, "frozen")
	assert.Equal(t, true, settings["post_apply"].(map[string]any)["enabled"])
}

func TestStarlarkStepsHookContextAndTemplateFallback(t *testing.T) {
	ctx := stepExecContext(&Hook{Kind: stepsKindName, OnFailure: OnFailureFail, With: []any{
		map[string]any{"type": "script", "name": "first", "interpreter": "starlark", "output": "none", "script": `output = ctx.component.settings["answer"]`},
		map[string]any{"type": "script", "interpreter": "starlark", "output": "none", "env": map[string]string{"PREVIOUS": "{{ .steps.first.value }}"}, "script": `def verify():
    if env["PREVIOUS"] != "42" or ctx.hook.event != "after.terraform.apply":
        fail("context or prior result missing")
verify()
`},
	}})
	ctx.Info = &schema.ConfigAndStacksInfo{ComponentFromArg: "api", Stack: "dev"}
	hooks := &Hooks{sections: map[string]any{"settings": map[string]any{"answer": 42}}}
	ctx.Hook.stepTemplateInfo = hooks.executionStackInfo(ctx.Info)
	ctx.Event, ctx.Outcome = AfterTerraformApply, Outcome{Status: RunSuccess}
	_, err := stepsEngine{}.Run(ctx)
	require.NoError(t, err)
}

func TestStarlarkHookWithoutComponent(t *testing.T) {
	for _, event := range []HookEvent{BeforeScaffoldGenerate, AfterTerraformApplyAggregate} {
		ctx := &ExecContext{Event: event}
		if event == AfterTerraformApplyAggregate {
			ctx.Info = &schema.ConfigAndStacksInfo{ComponentFromArg: "api", Stack: "dev"}
		}
		assert.Nil(t, scriptHookContext(ctx).Component)
	}
}

func TestStarlarkHookYAMLParallelContext(t *testing.T) {
	ctx := stepExecContext(&Hook{Kind: stepKindName, Type: "parallel", OnFailure: OnFailureFail, With: map[string]any{
		"steps": []any{map[string]any{
			"type": "script", "name": "check", "interpreter": "starlark", "script": `def verify():
    if ctx.hook.name != "parallel-hook" or ctx.component.name != "test-component":
        fail("hook context missing in YAML child")
verify()
`,
		}},
	}})
	ctx.HookName = "parallel-hook"
	_, err := stepEngine{}.Run(ctx)
	require.NoError(t, err)
}

func TestStarlarkHookComponentsGetUsesInjectedResolver(t *testing.T) {
	calls := 0
	resolver := func(_ context.Context, name, stack, componentType string) (*schema.ConfigAndStacksInfo, error) {
		calls++
		assert.Equal(t, []string{"api", "dev", "app"}, []string{name, stack, componentType})
		return &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"vars": map[string]any{"k": "v"}}}, nil
	}
	hook := &Hook{Kind: stepKindName, Type: "script", OnFailure: OnFailureFail, With: map[string]any{
		"interpreter": "starlark", "output": "none", "script": `
other = components.get("api", "dev", "app")
if other.vars["k"] != "v":
    fail("components.get did not use the resolved component")
`,
	}}
	ctx := stepExecContext(hook)
	ctx.AtmosConfig = &schema.AtmosConfiguration{BasePath: t.TempDir()}
	ctx.ComponentResolver = resolver

	_, err := stepEngine{}.Run(ctx)

	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestStarlarkHookComponentsGetWithoutAtmosConfigIsUnavailable(t *testing.T) {
	hook := &Hook{Kind: stepKindName, Type: "script", OnFailure: OnFailureFail, With: map[string]any{
		"interpreter": "starlark", "output": "none", "script": `components.get("api", "dev", "app")`,
	}}
	ctx := stepExecContext(hook)
	ctx.ComponentResolver = func(context.Context, string, string, string) (*schema.ConfigAndStacksInfo, error) {
		t.Fatal("the resolver must not be reached without an Atmos configuration")
		return nil, nil
	}

	_, err := stepEngine{}.Run(ctx)

	require.ErrorIs(t, err, errUtils.ErrStarlark)
}

func TestScriptComponentResolverDefaultsToExecutionPipeline(t *testing.T) {
	ctx := stepExecContext(&Hook{})
	ctx.AtmosConfig = &schema.AtmosConfiguration{BasePath: t.TempDir()}
	assert.NotNil(t, scriptComponentResolver(ctx), "the execution-pipeline resolver is the default")

	ctx.Info = nil
	assert.NotNil(t, scriptComponentResolver(ctx), "a missing Info must not panic")
}
