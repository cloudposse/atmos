package composition

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

type testCompositionProvider struct {
	componentType string
	commands      []string
}

func (p testCompositionProvider) GetType() string                                 { return p.componentType }
func (p testCompositionProvider) GetGroup() string                                { return "test" }
func (p testCompositionProvider) GetBasePath(_ *schema.AtmosConfiguration) string { return "" }
func (p testCompositionProvider) ListComponents(_ context.Context, _ string, _ map[string]any) ([]string, error) {
	return nil, nil
}
func (p testCompositionProvider) ValidateComponent(_ map[string]any) error    { return nil }
func (p testCompositionProvider) Execute(_ *component.ExecutionContext) error { return nil }
func (p testCompositionProvider) GenerateArtifacts(_ *component.ExecutionContext) error {
	return nil
}
func (p testCompositionProvider) GetAvailableCommands() []string { return p.commands }

func stackWithTypedComponents(componentType string, claims map[string]string) map[string]any {
	typeMap := map[string]any{}
	for componentName, compositionName := range claims {
		comp := map[string]any{}
		if compositionName != "" {
			comp["composition"] = compositionName
		}
		typeMap[componentName] = comp
	}
	return map[string]any{
		cfg.ComponentsSectionName: map[string]any{
			componentType: typeMap,
		},
	}
}

func withLifecycleStubs(
	t *testing.T,
	comps map[string]schema.Composition,
	stacksMap map[string]any,
	provider component.ComponentProvider,
	exec func(component.ComponentProvider, *component.ExecutionContext) error,
) {
	t.Helper()
	origInit, origDescribe := initCliConfig, describeStacks
	origGet, origExec := getComponentProvider, executeProvider
	t.Cleanup(func() {
		initCliConfig, describeStacks = origInit, origDescribe
		getComponentProvider, executeProvider = origGet, origExec
	})

	initCliConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{Compositions: comps}, nil
	}
	describeStacks = func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
		return stacksMap, nil
	}
	getComponentProvider = func(componentType string) (component.ComponentProvider, bool) {
		if provider == nil || provider.GetType() != componentType {
			return nil, false
		}
		return provider, true
	}
	if exec == nil {
		exec = func(_ component.ComponentProvider, _ *component.ExecutionContext) error { return nil }
	}
	executeProvider = exec
}

func TestResolveStatuses_ServiceOrderAndMissing(t *testing.T) {
	comps := map[string]schema.Composition{
		"storefront": {Description: "desc", Services: []string{"frontend", "api", "database"}},
	}
	stacksMap := map[string]any{
		"local": stackWithTypedComponents(cfg.ContainerComponentType, map[string]string{
			"api":      "storefront",
			"frontend": "storefront",
		}),
	}

	statuses, err := resolveStatuses(stacksMap, "local", "", comps)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, []string{"frontend", "api"}, memberComponents(statuses[0].Members))
	assert.Equal(t, []string{"frontend", "api"}, statuses[0].Fulfilled)
	assert.Equal(t, []string{"database"}, statuses[0].NotProvided)
}

func TestLifecycleTargets_AllCompositionsAndReverse(t *testing.T) {
	statuses := []status{
		{Name: "alpha", Members: []member{{Stack: "local", Component: "a1", ServiceIndex: 0}, {Stack: "local", Component: "a2", ServiceIndex: 1}}},
		{Name: "beta", Members: []member{{Stack: "local", Component: "b1", ServiceIndex: 0}}},
	}

	upTargets, err := lifecycleTargets(statuses, "", "up")
	require.NoError(t, err)
	assert.Equal(t, []string{"a1", "a2", "b1"}, memberComponents(upTargets))

	downTargets, err := lifecycleTargets(statuses, "", "down")
	require.NoError(t, err)
	assert.Equal(t, []string{"b1", "a2", "a1"}, memberComponents(downTargets))
}

func TestExecuteLifecycleRequiresStack(t *testing.T) {
	err := ExecuteLifecycle(context.Background(), &schema.ConfigAndStacksInfo{}, "up", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires --stack")
}

func TestExecuteLifecycleDispatchesAllCompositionMembersWithFlags(t *testing.T) {
	comps := map[string]schema.Composition{
		"storefront": {Services: []string{"frontend", "api", "database"}},
	}
	stacksMap := map[string]any{
		"local": stackWithTypedComponents(cfg.ContainerComponentType, map[string]string{
			"api":      "storefront",
			"frontend": "storefront",
		}),
	}
	provider := testCompositionProvider{componentType: cfg.ContainerComponentType, commands: []string{"logs"}}
	var calls []string
	var gotFlags map[string]any
	withLifecycleStubs(t, comps, stacksMap, provider, func(_ component.ComponentProvider, execCtx *component.ExecutionContext) error {
		calls = append(calls, execCtx.Stack+"/"+execCtx.ComponentType+"/"+execCtx.Component+"/"+execCtx.SubCommand)
		gotFlags = execCtx.Flags
		assert.Equal(t, execCtx.Component, execCtx.ConfigAndStacksInfo.ComponentFromArg)
		assert.Equal(t, execCtx.ComponentType, execCtx.ConfigAndStacksInfo.ComponentType)
		return nil
	})

	err := ExecuteLifecycle(context.Background(), &schema.ConfigAndStacksInfo{Stack: "local"}, "logs", "", map[string]any{"follow": true, "tail": "20"})
	require.NoError(t, err)
	assert.Equal(t, []string{"local/container/frontend/logs", "local/container/api/logs"}, calls)
	assert.Equal(t, true, gotFlags["follow"])
	assert.Equal(t, "20", gotFlags["tail"])
}

func TestExecuteLifecycleUnsupportedProviderCommand(t *testing.T) {
	comps := map[string]schema.Composition{
		"storefront": {Services: []string{"api"}},
	}
	stacksMap := map[string]any{
		"local": stackWithTypedComponents("mock", map[string]string{"api": "storefront"}),
	}
	provider := testCompositionProvider{componentType: "mock", commands: []string{"validate"}}
	withLifecycleStubs(t, comps, stacksMap, provider, nil)

	err := ExecuteLifecycle(context.Background(), &schema.ConfigAndStacksInfo{Stack: "local"}, "up", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `does not support "up"`)
}

func TestRunLifecycleTargetsContinuesAndAggregatesFailures(t *testing.T) {
	provider := testCompositionProvider{componentType: cfg.ContainerComponentType, commands: []string{"up"}}
	origGet, origExec := getComponentProvider, executeProvider
	t.Cleanup(func() {
		getComponentProvider, executeProvider = origGet, origExec
	})
	getComponentProvider = func(componentType string) (component.ComponentProvider, bool) {
		assert.Equal(t, cfg.ContainerComponentType, componentType)
		return provider, true
	}
	var calls []string
	executeProvider = func(_ component.ComponentProvider, execCtx *component.ExecutionContext) error {
		calls = append(calls, execCtx.Component)
		if execCtx.Component == "api" {
			return assert.AnError
		}
		return nil
	}

	targets := []member{
		{Stack: "local", Composition: "storefront", ComponentType: cfg.ContainerComponentType, Component: "frontend"},
		{Stack: "local", Composition: "storefront", ComponentType: cfg.ContainerComponentType, Component: "api"},
	}
	err := runLifecycleTargets(lifecycleRun{
		atmosConfig: &schema.AtmosConfiguration{},
		info:        &schema.ConfigAndStacksInfo{},
		verb:        "up",
		targets:     targets,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
	assert.Equal(t, []string{"frontend", "api"}, calls)
}
