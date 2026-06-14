package component

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

type graphTestProvider struct {
	calls []ExecutionContext
}

func (p *graphTestProvider) GetType() string { return cfg.KubernetesComponentType }

func (p *graphTestProvider) GetGroup() string { return "test" }

func (p *graphTestProvider) GetBasePath(*schema.AtmosConfiguration) string { return "" }

func (p *graphTestProvider) ListComponents(context.Context, string, map[string]any) ([]string, error) {
	return nil, nil
}

func (p *graphTestProvider) ValidateComponent(map[string]any) error { return nil }

func (p *graphTestProvider) Execute(ctx *ExecutionContext) error {
	p.calls = append(p.calls, *ctx)
	return nil
}

func (p *graphTestProvider) GenerateArtifacts(*ExecutionContext) error { return nil }

func (p *graphTestProvider) GetAvailableCommands() []string { return nil }

func TestBuildGraphIncludesDependenciesAndSkipsDisabledComponents(t *testing.T) {
	graph, err := BuildGraph(graphTestStacks(), cfg.KubernetesComponentType)
	require.NoError(t, err)

	require.Equal(t, 4, graph.Size())
	assert.Contains(t, graph.Nodes, GraphNodeID("base", "dev"))
	assert.Contains(t, graph.Nodes, GraphNodeID("api", "dev"))
	assert.Contains(t, graph.Nodes, GraphNodeID("base", "prod"))
	assert.Contains(t, graph.Nodes, GraphNodeID("worker", "dev"))
	assert.NotContains(t, graph.Nodes, GraphNodeID("abstract", "dev"))
	assert.NotContains(t, graph.Nodes, GraphNodeID("disabled", "dev"))

	api := graph.Nodes[GraphNodeID("api", "dev")]
	assert.Equal(t, []string{GraphNodeID("base", "dev")}, api.Dependencies)
	assert.Contains(t, graph.Nodes[GraphNodeID("base", "dev")].Dependents, GraphNodeID("api", "dev"))

	worker := graph.Nodes[GraphNodeID("worker", "dev")]
	assert.Equal(t, []string{GraphNodeID("base", "prod")}, worker.Dependencies)
	assert.Contains(t, graph.Nodes[GraphNodeID("base", "prod")].Dependents, GraphNodeID("worker", "dev"))
}

func TestBuildGraphSupportsLegacyDependsOn(t *testing.T) {
	stacks := map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.KubernetesComponentType: map[string]any{
					"base": map[string]any{},
					"api": map[string]any{
						cfg.SettingsSectionName: map[string]any{
							"depends_on": []any{"base", map[string]any{"component": "missing"}},
						},
					},
				},
			},
		},
	}

	graph, err := BuildGraph(stacks, cfg.KubernetesComponentType)
	require.NoError(t, err)

	api := graph.Nodes[GraphNodeID("api", "dev")]
	assert.Equal(t, []string{GraphNodeID("base", "dev")}, api.Dependencies)
}

func TestBuildGraphIgnoresMalformedStackSections(t *testing.T) {
	graph, err := BuildGraph(map[string]any{
		"string": "not a stack",
		"empty":  map[string]any{},
		"bad": map[string]any{
			cfg.ComponentsSectionName: "not components",
		},
	}, cfg.KubernetesComponentType)

	require.NoError(t, err)
	assert.Equal(t, 0, graph.Size())
}

func TestFilterGraph(t *testing.T) {
	graph, err := BuildGraph(graphTestStacks(), cfg.KubernetesComponentType)
	require.NoError(t, err)

	t.Run("nil graph returns empty graph", func(t *testing.T) {
		filtered := FilterGraph(nil, nil, nil)
		assert.NotNil(t, filtered)
		assert.Equal(t, 0, filtered.Size())
	})

	t.Run("stack filter keeps only selected stack", func(t *testing.T) {
		filtered := FilterGraph(graph, &schema.ConfigAndStacksInfo{Stack: "prod"}, nil)
		require.Equal(t, 1, filtered.Size())
		assert.Contains(t, filtered.Nodes, GraphNodeID("base", "prod"))
	})

	t.Run("selection includes dependencies and dependents", func(t *testing.T) {
		filtered := FilterGraph(graph, nil, &GraphSelection{
			NodeIDs:             []string{GraphNodeID("base", "dev"), GraphNodeID("base", "dev"), ""},
			IncludeDependencies: true,
			IncludeDependents:   true,
		})
		assert.Contains(t, filtered.Nodes, GraphNodeID("base", "dev"))
		assert.Contains(t, filtered.Nodes, GraphNodeID("api", "dev"))
		assert.NotContains(t, filtered.Nodes, GraphNodeID("base", "prod"))
	})
}

func TestExecuteGraphRunsComponentsInDependencyOrder(t *testing.T) {
	provider := &graphTestProvider{}

	err := ExecuteGraph(context.Background(), &GraphExecutionOptions{
		Provider:      provider,
		Info:          &schema.ConfigAndStacksInfo{},
		Stacks:        graphTestStacks(),
		ComponentType: cfg.KubernetesComponentType,
		SubCommand:    "apply",
		Flags:         map[string]any{"dry-run": true},
	})

	require.NoError(t, err)
	require.Len(t, provider.calls, 4)
	assertLessCallIndex(t, provider.calls, "base", "dev", "api", "dev")
	assertLessCallIndex(t, provider.calls, "base", "prod", "worker", "dev")
	for _, call := range provider.calls {
		assert.Equal(t, cfg.KubernetesComponentType, call.ComponentType)
		assert.Equal(t, "apply", call.SubCommand)
		assert.False(t, call.ConfigAndStacksInfo.All)
		assert.False(t, call.ConfigAndStacksInfo.Affected)
		assert.Equal(t, true, call.Flags["dry-run"])
	}
}

func TestExecuteGraphValidatesRequiredOptions(t *testing.T) {
	err := ExecuteGraph(context.Background(), &GraphExecutionOptions{Info: &schema.ConfigAndStacksInfo{}})
	require.ErrorContains(t, err, "component provider is nil")

	err = ExecuteGraph(context.Background(), &GraphExecutionOptions{Provider: &graphTestProvider{}})
	require.ErrorContains(t, err, "config and stacks info is nil")
}

func TestExecuteGraphRejectsNilArguments(t *testing.T) {
	err := ExecuteGraph(context.Background(), nil)
	require.ErrorIs(t, err, errUtils.ErrGraphExecutionOptions)

	//nolint:staticcheck // Intentionally passing a nil context to exercise the guard.
	err = ExecuteGraph(nil, &GraphExecutionOptions{Provider: &graphTestProvider{}, Info: &schema.ConfigAndStacksInfo{}})
	require.ErrorIs(t, err, errUtils.ErrGraphExecutionOptions)
}

func TestGraphNodeIDAvoidsDelimiterCollisions(t *testing.T) {
	// With a naive "%s-%s" format these two pairs would both yield "api-prod-dev".
	assert.NotEqual(t, GraphNodeID("api-prod", "dev"), GraphNodeID("api", "prod-dev"))
}

func TestFilterGraphSelectionValidatesSetEquality(t *testing.T) {
	graph, err := BuildGraph(graphTestStacks(), cfg.KubernetesComponentType)
	require.NoError(t, err)

	// A selection whose length equals the graph size but contains an unknown ID must
	// NOT short-circuit to the full graph; only the valid selected nodes survive.
	filtered := FilterGraph(graph, nil, &GraphSelection{
		NodeIDs: []string{
			GraphNodeID("base", "dev"),
			GraphNodeID("api", "dev"),
			GraphNodeID("worker", "dev"),
			GraphNodeID("does-not", "exist"),
		},
	})

	assert.Equal(t, 3, filtered.Size())
	assert.NotContains(t, filtered.Nodes, GraphNodeID("base", "prod"))
	assert.Contains(t, filtered.Nodes, GraphNodeID("base", "dev"))
}

func TestExecuteGraphHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	provider := &graphTestProvider{}
	err := ExecuteGraph(ctx, &GraphExecutionOptions{
		Provider:      provider,
		Info:          &schema.ConfigAndStacksInfo{},
		Stacks:        graphTestStacks(),
		ComponentType: cfg.KubernetesComponentType,
	})

	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, errUtils.ErrGraphExecutionCanceled)
	assert.Empty(t, provider.calls)
}

func TestLegacyDependsOnParsing(t *testing.T) {
	assert.Nil(t, parseLegacyDependsOn("invalid"))
	assert.Empty(t, parseLegacyDependsOn([]any{"", map[string]any{"stack": "dev"}, 123}))

	deps := parseLegacyDependsOn(map[string]any{
		"first": "base",
		"second": map[string]any{
			"component": "database",
			"stack":     "prod",
		},
	})

	assert.ElementsMatch(t, []schema.ComponentDependency{
		{Component: "base"},
		{Component: "database", Stack: "prod"},
	}, deps)
}

func TestSortedUniqueStrings(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, sortedUniqueStrings([]string{"b", "", "a", "b"}))
}

func graphTestStacks() map[string]any {
	return map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.KubernetesComponentType: map[string]any{
					"base": map[string]any{},
					"api": map[string]any{
						cfg.DependenciesSectionName: map[string]any{
							"components": []any{
								map[string]any{"name": "base"},
								map[string]any{"name": "other", "kind": "terraform"},
							},
						},
					},
					"worker": map[string]any{
						cfg.DependenciesSectionName: map[string]any{
							"components": []any{
								map[string]any{"name": "base", "stack": "prod"},
							},
						},
					},
					"abstract": map[string]any{
						cfg.MetadataSectionName: map[string]any{"type": "abstract"},
					},
					"disabled": map[string]any{
						cfg.MetadataSectionName: map[string]any{"enabled": false},
					},
				},
			},
		},
		"prod": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.KubernetesComponentType: map[string]any{
					"base": map[string]any{},
				},
			},
		},
	}
}

func assertLessCallIndex(t *testing.T, calls []ExecutionContext, beforeComponent, beforeStack, afterComponent, afterStack string) {
	t.Helper()

	before := -1
	after := -1
	for i := range calls {
		call := &calls[i]
		if call.Component == beforeComponent && call.Stack == beforeStack {
			before = i
		}
		if call.Component == afterComponent && call.Stack == afterStack {
			after = i
		}
	}
	require.NotEqual(t, -1, before)
	require.NotEqual(t, -1, after)
	assert.Less(t, before, after)
}
