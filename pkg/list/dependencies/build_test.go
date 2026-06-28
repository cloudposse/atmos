package dependencies

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// terraformStacks is a small helper to build a stacks map of the shape produced
// by `describe stacks`: stack -> components -> terraform -> component -> section.
func terraformStacks(components map[string]map[string]map[string]any) map[string]any {
	stacks := make(map[string]any)
	for stack, comps := range components {
		tf := make(map[string]any)
		for name, section := range comps {
			tf[name] = any(section)
		}
		stacks[stack] = map[string]any{
			"components": map[string]any{
				"terraform": tf,
			},
		}
	}
	return stacks
}

func dependsOn(entries ...map[string]any) map[string]any {
	deps := make(map[string]any, len(entries))
	for i, e := range entries {
		deps[string(rune('1'+i))] = e
	}
	return map[string]any{"settings": map[string]any{"depends_on": deps}}
}

func TestBuildGraph_SettingsDependsOn(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": {},
			"app": dependsOn(map[string]any{"component": "vpc"}),
		},
	})

	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	require.Equal(t, 2, graph.Size())

	app, ok := graph.GetNode(NodeID("app", "dev"))
	require.True(t, ok)
	assert.Equal(t, []string{NodeID("vpc", "dev")}, app.Dependencies)

	vpc, ok := graph.GetNode(NodeID("vpc", "dev"))
	require.True(t, ok)
	assert.Equal(t, []string{NodeID("app", "dev")}, vpc.Dependents)
}

func TestBuildGraph_DependenciesComponents(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"app": {},
			"web": {
				"dependencies": map[string]any{
					"components": []any{
						map[string]any{"component": "app"},
					},
				},
			},
		},
	})

	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	web, ok := graph.GetNode(NodeID("web", "dev"))
	require.True(t, ok)
	assert.Equal(t, []string{NodeID("app", "dev")}, web.Dependencies)
}

func TestBuildGraph_CrossStackDependency(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev":  {"app": dependsOn(map[string]any{"component": "vpc", "stack": "prod"})},
		"prod": {"vpc": {}},
	})

	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	app, ok := graph.GetNode(NodeID("app", "dev"))
	require.True(t, ok)
	assert.Equal(t, []string{NodeID("vpc", "prod")}, app.Dependencies)
}

func TestBuildGraph_SkipsAbstractAndDisabled(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"real":     {},
			"abstract": {"metadata": map[string]any{"type": "abstract"}},
			"disabled": {"metadata": map[string]any{"enabled": false}},
		},
	})

	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	require.Equal(t, 1, graph.Size())
	_, ok := graph.GetNode(NodeID("real", "dev"))
	assert.True(t, ok)
	_, ok = graph.GetNode(NodeID("abstract", "dev"))
	assert.False(t, ok)
	_, ok = graph.GetNode(NodeID("disabled", "dev"))
	assert.False(t, ok)
}

func TestBuildGraph_SkipsMissingTarget(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {"app": dependsOn(map[string]any{"component": "ghost"})},
	})

	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	app, ok := graph.GetNode(NodeID("app", "dev"))
	require.True(t, ok)
	assert.Empty(t, app.Dependencies)
}

func TestBuildGraph_ToleratesCycles(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"a": dependsOn(map[string]any{"component": "b"}),
			"b": dependsOn(map[string]any{"component": "a"}),
		},
	})

	// Unlike the execution builder, BuildGraph must not fail on cycles.
	graph, err := BuildGraph(stacks)
	require.NoError(t, err)
	require.Equal(t, 2, graph.Size())

	hasCycle, _ := graph.HasCycles()
	assert.True(t, hasCycle)
}
