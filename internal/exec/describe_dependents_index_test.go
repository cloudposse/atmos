package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestBuildDependencyIndex_Empty(t *testing.T) {
	idx := buildDependencyIndex(map[string]any{})
	assert.Empty(t, idx, "empty stacks should produce empty index")
}

func TestBuildDependencyIndex_NoDependencies(t *testing.T) {
	stacks := map[string]any{
		"dev-use1": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{"region": "us-east-1"},
					},
				},
			},
		},
	}
	idx := buildDependencyIndex(stacks)
	assert.Empty(t, idx, "components without depends_on should produce empty index")
}

func TestBuildDependencyIndex_WithDependencies(t *testing.T) {
	stacks := map[string]any{
		"dev-use1": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"eks-cluster": map[string]any{
						"vars": map[string]any{
							"namespace":   "acme",
							"tenant":      "dev",
							"environment": "use1",
							"stage":       "compute",
						},
						"dependencies": map[string]any{
							"components": []any{
								map[string]any{"component": "vpc"},
							},
						},
					},
					"rds": map[string]any{
						"vars": map[string]any{
							"namespace":   "acme",
							"tenant":      "dev",
							"environment": "use1",
							"stage":       "data",
						},
						"dependencies": map[string]any{
							"components": []any{
								map[string]any{"component": "vpc"},
								map[string]any{"component": "eks-cluster"},
							},
						},
					},
				},
			},
		},
	}

	idx := buildDependencyIndex(stacks)

	// "vpc" should have 2 dependents: eks-cluster and rds.
	require.Len(t, idx["vpc"], 2, "vpc should have 2 dependents")
	names := []string{idx["vpc"][0].StackComponentName, idx["vpc"][1].StackComponentName}
	assert.Contains(t, names, "eks-cluster")
	assert.Contains(t, names, "rds")

	// "eks-cluster" should have 1 dependent: rds.
	require.Len(t, idx["eks-cluster"], 1)
	assert.Equal(t, "rds", idx["eks-cluster"][0].StackComponentName)

	// "rds" should have no dependents.
	assert.Empty(t, idx["rds"])
}

func TestBuildDependencyIndex_SkipsAbstractComponents(t *testing.T) {
	stacks := map[string]any{
		"dev-use1": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"abstract-component": map[string]any{
						"metadata": map[string]any{"type": "abstract"},
						"vars":     map[string]any{"region": "us-east-1"},
						"dependencies": map[string]any{
							"components": []any{
								map[string]any{"component": "vpc"},
							},
						},
					},
				},
			},
		},
	}
	idx := buildDependencyIndex(stacks)
	assert.Empty(t, idx["vpc"], "abstract components should be skipped")
}

func TestBuildDependencyIndex_MultipleStacks(t *testing.T) {
	stacks := map[string]any{
		"dev-use1": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"app": map[string]any{
						"vars": map[string]any{"tenant": "dev", "environment": "use1"},
						"dependencies": map[string]any{
							"components": []any{
								map[string]any{"component": "vpc"},
							},
						},
					},
				},
			},
		},
		"prod-use1": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"app": map[string]any{
						"vars": map[string]any{"tenant": "prod", "environment": "use1"},
						"dependencies": map[string]any{
							"components": []any{
								map[string]any{"component": "vpc"},
							},
						},
					},
				},
			},
		},
	}
	idx := buildDependencyIndex(stacks)
	require.Len(t, idx["vpc"], 2, "vpc should have dependents from both stacks")
	stackNames := []string{idx["vpc"][0].StackName, idx["vpc"][1].StackName}
	assert.Contains(t, stackNames, "dev-use1")
	assert.Contains(t, stackNames, "prod-use1")
}

func TestFindComponentSectionInCachedStacks(t *testing.T) {
	stacks := map[string]any{
		"dev-use1": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{"cidr": "10.0.0.0/16"},
					},
				},
			},
		},
	}

	// Found.
	section := findComponentSectionInCachedStacks(stacks, "dev-use1", "vpc")
	require.NotNil(t, section)
	vars := section["vars"].(map[string]any)
	assert.Equal(t, "10.0.0.0/16", vars["cidr"])

	// Not found — wrong stack.
	assert.Nil(t, findComponentSectionInCachedStacks(stacks, "prod-use1", "vpc"))

	// Not found — wrong component.
	assert.Nil(t, findComponentSectionInCachedStacks(stacks, "dev-use1", "rds"))
}

func TestFindDependentsFromIndex_NoMatches(t *testing.T) {
	args := &DescribeDependentsArgs{Component: "vpc", Stack: "dev-use1", DepIndex: dependencyIndex{}}
	providedVars := &schema.Context{Namespace: "acme", Tenant: "dev"}

	result := findDependentsFromIndex(nil, args, providedVars)
	assert.Nil(t, result, "no index entries should return nil")
}

func TestFindComponentSectionInCachedStacks_Helmfile(t *testing.T) {
	stacks := map[string]any{
		"dev-use1": map[string]any{
			"components": map[string]any{
				"helmfile": map[string]any{
					"nginx": map[string]any{
						"vars": map[string]any{"chart": "nginx"},
					},
				},
			},
		},
	}
	// Found via helmfile path.
	section := findComponentSectionInCachedStacks(stacks, "dev-use1", "nginx")
	require.NotNil(t, section)
	assert.Equal(t, "nginx", section["vars"].(map[string]any)["chart"])
}

func TestFindComponentSectionInCachedStacks_InvalidStackSection(t *testing.T) {
	// Stack section is not a map.
	stacks := map[string]any{"bad": "not-a-map"}
	assert.Nil(t, findComponentSectionInCachedStacks(stacks, "bad", "vpc"))

	// Components section is not a map.
	stacks2 := map[string]any{"dev": map[string]any{"components": "not-a-map"}}
	assert.Nil(t, findComponentSectionInCachedStacks(stacks2, "dev", "vpc"))
}

func TestFindDependentsByScan_SkipsAbstractAndSelf(t *testing.T) {
	stacks := map[string]any{
		"dev-use1": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					// Self — should be skipped.
					"vpc": map[string]any{
						"vars": map[string]any{"tenant": "dev"},
						"dependencies": map[string]any{
							"components": []any{
								map[string]any{"component": "vpc"},
							},
						},
					},
					// Abstract — should be skipped.
					"abstract-thing": map[string]any{
						"metadata": map[string]any{"type": "abstract"},
						"vars":     map[string]any{"tenant": "dev"},
						"dependencies": map[string]any{
							"components": []any{
								map[string]any{"component": "vpc"},
							},
						},
					},
					// Valid dependent.
					"app": map[string]any{
						"vars": map[string]any{"tenant": "dev", "environment": "use1"},
						"dependencies": map[string]any{
							"components": []any{
								map[string]any{"component": "vpc"},
							},
						},
					},
					// No vars — should be skipped.
					"no-vars": map[string]any{
						"dependencies": map[string]any{
							"components": []any{
								map[string]any{"component": "vpc"},
							},
						},
					},
					// No dependencies — should be skipped.
					"no-deps": map[string]any{
						"vars": map[string]any{"tenant": "dev"},
					},
					// Invalid component map — should be skipped.
					"bad": "not-a-map",
				},
				// Invalid component type section — should be skipped.
				"invalid-type": "not-a-map",
			},
		},
		// Invalid stack section — should be skipped.
		"bad-stack": "not-a-map",
		// Missing components — should be skipped.
		"empty-stack": map[string]any{"no-components": true},
	}

	args := &DescribeDependentsArgs{
		Component: "vpc",
		Stack:     "dev-use1",
	}
	providedVars := &schema.Context{Tenant: "dev"}

	deps, err := findDependentsByScan(nil, args, stacks, providedVars)
	require.NoError(t, err)
	require.Len(t, deps, 1, "only 'app' should be a valid dependent")
	assert.Equal(t, "app", deps[0].Component)
	assert.Equal(t, "dev-use1", deps[0].Stack)
}

func TestFindDependentsFromIndex_SkipsSelfReference(t *testing.T) {
	idx := dependencyIndex{
		"vpc": {
			{
				StackName:          "dev-use1",
				StackComponentName: "vpc", // Self-reference.
				StackComponentType: "terraform",
				StackComponentVars: schema.Context{Tenant: "dev"},
				DepSource:          dependencySourceDependenciesComponents,
				DependsOn:          schema.ComponentDependency{Component: "vpc"},
			},
		},
	}
	args := &DescribeDependentsArgs{Component: "vpc", Stack: "dev-use1", DepIndex: idx}
	providedVars := &schema.Context{Tenant: "dev"}

	result := findDependentsFromIndex(nil, args, providedVars)
	assert.Empty(t, result, "self-references should be skipped")
}
