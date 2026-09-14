package dependencies

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
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

func TestBuildGraphDefersRequiredValuesWithConfiguredDelimiter(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": {},
			"app": {
				"dependencies": map[string]any{
					"components": []any{
						map[string]any{"name": "vpc", "required": "[[ .dependencyRequired ]]"},
					},
				},
			},
		},
	})

	graph, err := BuildGraph(stacks, "[[")
	require.NoError(t, err)
	assert.Contains(t, graph.Nodes[NodeID("app", "dev")].Dependencies, NodeID("vpc", "dev"))
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

func TestRequiredDependencySourcesMatchesTargetType(t *testing.T) {
	stacks := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{"image": map[string]any{}},
				"packer": map[string]any{
					"image": map[string]any{},
					"builder": map[string]any{
						"dependencies": map[string]any{
							"components": []any{map[string]any{"name": "image", "kind": "packer"}},
						},
					},
				},
			},
		},
	}

	terraformSources := RequiredDependencySources(stacks, []rootTarget{{component: "image", componentType: "terraform", stack: "dev"}}, "")
	packerSources := RequiredDependencySources(stacks, []rootTarget{{component: "image", componentType: "packer", stack: "dev"}}, "")

	assert.Empty(t, terraformSources)
	assert.Equal(t, map[string][]string{"dev": {"builder"}}, packerSources)
}

func TestRequiredDependencySourcesHonorsConfiguredDelimiter(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"image": {},
			"app": {"dependencies": map[string]any{"components": []any{map[string]any{
				"name":     "image",
				"required": "[[ .dependencyRequired ]]",
			}}}},
		},
	})

	sources := RequiredDependencySources(stacks, []rootTarget{{component: "image", componentType: "terraform", stack: "dev"}}, "[[")

	assert.Equal(t, map[string][]string{"dev": {"app"}}, sources)
}

func TestBuildGraph_CrossTypeDependency(t *testing.T) {
	stacks := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{},
				},
				"helmfile": map[string]any{
					"nginx": map[string]any{
						"dependencies": map[string]any{
							"components": []any{
								map[string]any{"component": "vpc", "kind": "terraform"},
							},
						},
					},
				},
			},
		},
	}

	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	for _, node := range graph.Nodes {
		if node.Component != "nginx" {
			continue
		}
		assert.Equal(t, "helmfile", node.Type)
		assert.Equal(t, []string{NodeID("vpc", "dev")}, node.Dependencies)
		return
	}
	t.Fatal("helmfile nginx node is missing")
}

func TestBuildGraph_DependenciesComponentsEmptyPreventsSettingsFallback(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": {},
			"app": {
				"dependencies": map[string]any{
					"components": []any{},
				},
				"settings": map[string]any{
					"depends_on": map[string]any{
						"1": map[string]any{"component": "vpc"},
					},
				},
			},
		},
	})

	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	app, ok := graph.GetNode(NodeID("app", "dev"))
	require.True(t, ok)
	assert.Empty(t, app.Dependencies, "explicit dependencies.components must not fall back to settings.depends_on")
}

func TestBuildGraph_DependenciesComponentsInvalidFails(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": {},
			"app": {
				"dependencies": map[string]any{
					"components": "not-a-list",
				},
				"settings": map[string]any{
					"depends_on": map[string]any{
						"1": map[string]any{"component": "vpc"},
					},
				},
			},
		},
	})

	_, err := BuildGraph(stacks)
	require.ErrorIs(t, err, errUtils.ErrDependencyResolution)
}

func TestBuildGraph_InvalidRequiredValueFails(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": {},
			"app": {
				"dependencies": map[string]any{
					"components": []any{
						map[string]any{"component": "vpc", "required": "sometimes"},
					},
				},
			},
		},
	})

	_, err := BuildGraph(stacks)
	require.ErrorIs(t, err, schema.ErrComponentDependencyInvalidRequired)
}

func TestBuildGraph_DefersUnresolvedRequiredTemplate(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": {},
			"app": {
				"dependencies": map[string]any{
					"components": []any{
						map[string]any{"component": "vpc", "required": "{{ .vars.vpc_required }}"},
					},
				},
			},
		},
	})

	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	app, ok := graph.GetNode(NodeID("app", "dev"))
	require.True(t, ok)
	assert.Equal(t, []string{NodeID("vpc", "dev")}, app.Dependencies)
}

func TestBuildGraph_IgnoresMalformedStackShapes(t *testing.T) {
	stacks := map[string]any{
		"stack-is-not-map": "bad",
		"components-is-not-map": map[string]any{
			"components": "bad",
		},
		"terraform-is-not-map": map[string]any{
			"components": map[string]any{
				"terraform": "bad",
			},
		},
		"component-is-not-map": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"bad": "bad",
				},
			},
		},
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"app": map[string]any{},
				},
			},
		},
	}

	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	require.Equal(t, 1, graph.Size())
	_, ok := graph.GetNode(NodeID("app", "dev"))
	assert.True(t, ok)
}

func TestBuildGraph_FiltersPathDependencies(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"app": {
				"dependencies": map[string]any{
					"components": []any{
						map[string]any{"kind": "file", "path": "main.tf"},
						map[string]any{"kind": "folder", "path": "modules"},
					},
					"files":   []string{"values.yaml"},
					"folders": []string{"templates"},
				},
			},
		},
	})

	graph, err := BuildGraph(stacks)
	require.NoError(t, err)

	app, ok := graph.GetNode(NodeID("app", "dev"))
	require.True(t, ok)
	assert.Empty(t, app.Dependencies)
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

func TestBuildGraph_FailsForRequiredUnavailableTarget(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		targetBody map[string]any
		wantErr    error
	}{
		{name: "missing", target: "missing", wantErr: errUtils.ErrDependencyTargetNotFound},
		{name: "abstract", target: "abstract", targetBody: map[string]any{"metadata": map[string]any{"type": "abstract"}}, wantErr: errUtils.ErrDependencyTargetNotFound},
		{name: "disabled", target: "disabled", targetBody: map[string]any{"metadata": map[string]any{"enabled": false}}, wantErr: errUtils.ErrDependencyTargetUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			components := map[string]map[string]any{
				"app": {
					"dependencies": map[string]any{"components": []any{map[string]any{"component": test.target}}},
				},
			}
			if test.targetBody != nil {
				components[test.target] = test.targetBody
			}
			_, err := BuildGraph(terraformStacks(map[string]map[string]map[string]any{"dev": components}))
			require.ErrorIs(t, err, test.wantErr)
		})
	}
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

func TestBuildGraph_MalformedDependenciesSectionFails(t *testing.T) {
	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": {},
			"app": {
				"dependencies": "not-a-map",
				"settings": map[string]any{
					"depends_on": map[string]any{"1": map[string]any{"component": "vpc"}},
				},
			},
		},
	})

	_, err := BuildGraph(stacks)
	require.ErrorIs(t, err, errUtils.ErrDependencyResolution)
	require.ErrorIs(t, err, errUtils.ErrInvalidDependenciesSection)
}
