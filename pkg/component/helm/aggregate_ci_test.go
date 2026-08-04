package helm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestHelmBulkCICollectorRecordsAndSortsResults(t *testing.T) {
	collector := newHelmBulkCICollector("plan")
	collector.setSummary(&schema.ConfigAndStacksInfo{Stack: "prod", ComponentFromArg: "api"}, map[string]any{"diff": "+ change"}, nil)
	collector.finish(&component.ExecutionContext{Stack: "prod", Component: "api"}, time.Unix(1, 0), time.Unix(1, int64(25*time.Millisecond)), nil)

	sentinel := errors.New("render failed")
	collector.finish(&component.ExecutionContext{Stack: "dev", Component: "db"}, time.Unix(2, 0), time.Unix(2, int64(time.Millisecond)), sentinel)

	resultSet := collector.resultSet()
	assert.Equal(t, "plan", resultSet.Command)
	require.Len(t, resultSet.Results, 2)
	assert.Equal(t, "db", resultSet.Results[0].Component)
	assert.False(t, resultSet.Results[0].Processed)
	assert.Equal(t, "failed", resultSet.Results[0].Status)
	assert.Equal(t, "render failed", resultSet.Results[0].Error)
	assert.Equal(t, "api", resultSet.Results[1].Component)
	assert.True(t, resultSet.Results[1].Processed)
	assert.Equal(t, int64(25), resultSet.Results[1].DurationMS)
	assert.Equal(t, "+ change", resultSet.Results[1].Summary["diff"])

	resultSet.Results[1].Summary["diff"] = "mutated"
	assert.Equal(t, "+ change", collector.resultSet().Results[1].Summary["diff"])
}

func TestHelmBulkCICollectorDeepCopiesNestedSummary(t *testing.T) {
	collector := newHelmBulkCICollector("apply")
	wait := map[string]any{"strategy": "watcher"}
	kinds := []string{"Deployment", "Service"}
	summary := map[string]any{
		"release":      map[string]any{"wait": wait},
		"object_kinds": kinds,
	}
	collector.setSummary(&schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "api"}, summary, nil)

	wait["strategy"] = "legacy"
	kinds[0] = "Job"
	result := collector.resultSet().Results[0]
	release := result.Summary["release"].(map[string]any)
	assert.Equal(t, "watcher", release["wait"].(map[string]any)["strategy"])
	assert.Equal(t, []string{"Deployment", "Service"}, result.Summary["object_kinds"])

	release["wait"].(map[string]any)["strategy"] = "mutated"
	result.Summary["object_kinds"].([]string)[1] = "ConfigMap"
	retained := collector.resultSet().Results[0].Summary
	assert.Equal(t, "watcher", retained["release"].(map[string]any)["wait"].(map[string]any)["strategy"])
	assert.Equal(t, []string{"Deployment", "Service"}, retained["object_kinds"])
}

type helmBulkGraphTestProvider struct {
	failComponent string
}

func (p *helmBulkGraphTestProvider) GetType() string                               { return cfg.HelmComponentType }
func (p *helmBulkGraphTestProvider) GetGroup() string                              { return "test" }
func (p *helmBulkGraphTestProvider) GetBasePath(*schema.AtmosConfiguration) string { return "" }
func (p *helmBulkGraphTestProvider) ListComponents(context.Context, string, map[string]any) ([]string, error) {
	return nil, nil
}
func (p *helmBulkGraphTestProvider) ValidateComponent(map[string]any) error { return nil }
func (p *helmBulkGraphTestProvider) Execute(ctx *component.ExecutionContext) error {
	if ctx.Component == p.failComponent {
		return errors.New("operation failed")
	}
	return nil
}
func (p *helmBulkGraphTestProvider) GenerateArtifacts(*component.ExecutionContext) error { return nil }
func (p *helmBulkGraphTestProvider) GetAvailableCommands() []string                      { return nil }

func TestHelmBulkCICollectorRecordsDependencyBlockedComponents(t *testing.T) {
	collector := newHelmBulkCICollector("apply")
	provider := &bulkCollectingProvider{
		ComponentProvider: &helmBulkGraphTestProvider{failComponent: "base"},
		collector:         collector,
	}
	stacks := map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.HelmComponentType: map[string]any{
					"base": map[string]any{},
					"api": map[string]any{
						cfg.SettingsSectionName: map[string]any{"depends_on": []any{"base"}},
					},
				},
			},
		},
	}

	err := component.ExecuteGraph(context.Background(), &component.GraphExecutionOptions{
		Provider:      provider,
		Info:          &schema.ConfigAndStacksInfo{},
		Stacks:        stacks,
		ComponentType: cfg.HelmComponentType,
		SubCommand:    "apply",
	})
	require.Error(t, err)

	results := collector.resultSet().Results
	require.Len(t, results, 2)
	assert.Equal(t, "api", results[0].Component)
	assert.Equal(t, "skipped", results[0].Status)
	assert.False(t, results[0].Processed)
	assert.Equal(t, "base", results[1].Component)
	assert.Equal(t, "failed", results[1].Status)
}

func TestHelmBulkCICollectorUsesProcessedComponentIdentity(t *testing.T) {
	collector := newHelmBulkCICollector("plan")
	info := schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "apps/app"}
	collector.setSummary(&info, map[string]any{"diff": "+ change"}, nil)
	collector.finish(&component.ExecutionContext{
		Stack:               "dev",
		Component:           "app",
		ConfigAndStacksInfo: info,
	}, time.Unix(1, 0), time.Unix(1, int64(time.Millisecond)), nil)

	results := collector.resultSet().Results
	require.Len(t, results, 1)
	assert.Equal(t, "apps/app", results[0].Component)
	assert.True(t, results[0].Processed)
	assert.Equal(t, int64(1), results[0].DurationMS)
	assert.Equal(t, "+ change", results[0].Summary["diff"])
}

func TestRunWithHooksBulkCollectorSuppressesPerComponentCI(t *testing.T) {
	originalHooks := getHooks
	originalApply := applyHelmRelease
	originalCI := runCIHooks
	t.Cleanup(func() {
		getHooks = originalHooks
		applyHelmRelease = originalApply
		runCIHooks = originalCI
	})

	getHooks = func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (*hooks.Hooks, error) {
		return &hooks.Hooks{}, nil
	}
	applyHelmRelease = func(context.Context, *chartSpec, bool) (releaseActionResult, error) {
		return releaseActionResult{Manifest: helmExecutorManifest, Operation: releaseOperationInstall}, nil
	}
	ciCalls := 0
	runCIHooks = func(*hooks.RunCIHooksOptions) error {
		ciCalls++
		return nil
	}

	collector := newHelmBulkCICollector("apply")
	ctx := &component.ExecutionContext{Flags: map[string]any{helmBulkCICollectorFlag: collector}}
	info := &schema.ConfigAndStacksInfo{
		Stack: "dev", ComponentFromArg: "api", SubCommand: "apply",
		ComponentSection: map[string]any{"chart": "api", "name": "api"},
	}
	require.NoError(t, runWithHooks(ctx, &schema.AtmosConfiguration{}, info, OperationApply, ""))
	assert.Zero(t, ciCalls)
	results := collector.resultSet().Results
	require.Len(t, results, 1)
	assert.True(t, results[0].Processed)
	assert.Equal(t, "api", results[0].Summary["chart"])
}

func TestExecuteBulkEmitsOneAggregateCIHook(t *testing.T) {
	originalDescribe := executeDescribeStacks
	originalGraph := executeGraph
	originalCI := runCIHooks
	t.Cleanup(func() {
		executeDescribeStacks = originalDescribe
		executeGraph = originalGraph
		runCIHooks = originalCI
	})

	executeDescribeStacks = func(
		*schema.AtmosConfiguration,
		string,
		[]string,
		[]string,
		[]string,
		bool,
		bool,
		bool,
		bool,
		[]string,
		auth.AuthManager,
	) (map[string]any, error) {
		return map[string]any{"dev": map[string]any{}}, nil
	}
	sentinel := errors.New("graph failed")
	executeGraph = func(_ context.Context, opts *component.GraphExecutionOptions) error {
		assert.Equal(t, "plan", opts.SubCommand)
		_, wrapped := opts.Provider.(*bulkCollectingProvider)
		assert.True(t, wrapped)
		collector, ok := opts.Flags[helmBulkCICollectorFlag].(*helmBulkCICollector)
		require.True(t, ok)
		collector.setSummary(&schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "api"}, map[string]any{"diff": "+ change"}, nil)
		collector.finish(&component.ExecutionContext{Stack: "dev", Component: "api"}, time.Now(), time.Now(), sentinel)
		return sentinel
	}

	var captured *hooks.RunCIHooksOptions
	runCIHooks = func(opts *hooks.RunCIHooksOptions) error {
		captured = opts
		return nil
	}

	ctx := &component.ExecutionContext{Flags: map[string]any{"ci": true}}
	info := &schema.ConfigAndStacksInfo{All: true, SubCommand: "plan"}
	require.ErrorIs(t, executeBulk(ctx, &schema.AtmosConfiguration{}, info, OperationDiff), sentinel)
	require.NotNil(t, captured)
	assert.Equal(t, hooks.AfterHelmPlanAggregate, captured.Event)
	assert.True(t, captured.ForceCIMode)
	assert.ErrorIs(t, captured.CommandError, sentinel)
	resultSet, ok := captured.Aggregate.(schema.HelmCIResultSet)
	require.True(t, ok)
	assert.Equal(t, "plan", resultSet.Command)
	require.Len(t, resultSet.Results, 1)
	assert.Equal(t, "graph failed", resultSet.Results[0].Error)
	_, retained := ctx.Flags[helmBulkCICollectorFlag]
	assert.False(t, retained)
}

func TestHelmAggregateCIHelpers(t *testing.T) {
	assert.True(t, supportsHelmAggregateCI("plan"))
	assert.True(t, supportsHelmAggregateCI("apply"))
	assert.False(t, supportsHelmAggregateCI("template"))
	assert.Nil(t, helmBulkCollector(nil))
	assert.Nil(t, helmBulkCollector(&component.ExecutionContext{}))
}
