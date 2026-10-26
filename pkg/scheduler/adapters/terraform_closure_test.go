package adapters

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// terraformClosureTestStacks builds a three-level dependency chain in stack
// "dev" — app (tags: app) -> database (tags: data) -> vpc (tags: network) —
// so closure expansion across tag boundaries can be asserted precisely.
func terraformClosureTestStacks() map[string]any {
	component := func(tags []any, dependsOn map[string]any) map[string]any {
		section := map[string]any{
			cfg.MetadataSectionName: map[string]any{
				"component": "mock",
				"tags":      tags,
			},
			"vars": map[string]any{},
		}
		if dependsOn != nil {
			section[cfg.SettingsSectionName] = map[string]any{"depends_on": dependsOn}
		}
		return section
	}

	return map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"vpc":      component([]any{"network"}, nil),
					"database": component([]any{"data"}, map[string]any{"1": map[string]any{"component": "vpc"}}),
					"app":      component([]any{"app"}, map[string]any{"1": map[string]any{"component": "database"}}),
				},
			},
		},
	}
}

// terraformCrossStackClosureTestStacks builds a dependency chain that spans
// two stacks — app in stack "dev" depends on vpc in stack "core" via the
// modern dependencies.components declaration — so cross-stack closure
// expansion and ordering can be asserted precisely.
func terraformCrossStackClosureTestStacks() map[string]any {
	return map[string]any{
		"core": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"vpc": map[string]any{
						cfg.MetadataSectionName: map[string]any{"component": "mock"},
						"vars":                  map[string]any{},
					},
				},
			},
		},
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"app": map[string]any{
						cfg.MetadataSectionName: map[string]any{"component": "mock"},
						"vars":                  map[string]any{},
						cfg.DependenciesSectionName: map[string]any{
							"components": []any{
								map[string]any{"component": "vpc", "stack": "core"},
							},
						},
					},
				},
			},
		},
	}
}

// executedClosureComponents runs ExecuteTerraform over the closure fixture and
// returns component names in execution order (concurrency is 1, so the order
// is the scheduler's dependency order).
func executedClosureComponents(t *testing.T, info *schema.ConfigAndStacksInfo, selection *TerraformSelection) []string {
	t.Helper()

	var executed []string
	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info:        info,
		Stacks:      terraformClosureTestStacks(),
		Selection:   selection,
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			executed = append(executed, execution.Info.Component)
			return TerraformExecutionResult{}, nil
		},
	})
	require.NoError(t, err)
	return executed
}

// executedClosureNodes runs ExecuteTerraform over the given stacks fixture and
// returns "component-stack" node IDs in execution order (concurrency is 1, so
// the order is the scheduler's dependency order).
func executedClosureNodes(t *testing.T, stacks map[string]any, info *schema.ConfigAndStacksInfo) []string {
	t.Helper()

	var executed []string
	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info:        info,
		Stacks:      stacks,
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			executed = append(executed, terraformNodeID(execution.Info.Component, execution.Info.Stack))
			return TerraformExecutionResult{}, nil
		},
	})
	require.NoError(t, err)
	return executed
}

// TestExecuteTerraformIncludeDependenciesExpandsAcrossSelectors verifies that included dependencies expand beyond the selector matches in dependency order.
func TestExecuteTerraformIncludeDependenciesExpandsAcrossSelectors(t *testing.T) {
	t.Run("tags seed keeps non-matching prerequisites in dependency order", func(t *testing.T) {
		executed := executedClosureComponents(t, &schema.ConfigAndStacksInfo{
			SubCommand:          "plan",
			Tags:                []string{"app"},
			IncludeDependencies: -1,
		}, nil)
		require.Equal(t, []string{"vpc", "database", "app"}, executed)
	})

	t.Run("depth 1 stops one dependency level deep", func(t *testing.T) {
		executed := executedClosureComponents(t, &schema.ConfigAndStacksInfo{
			SubCommand:          "plan",
			Tags:                []string{"app"},
			IncludeDependencies: 1,
		}, nil)
		require.Equal(t, []string{"database", "app"}, executed)
	})

	t.Run("without closure flags tags filtering is unchanged", func(t *testing.T) {
		executed := executedClosureComponents(t, &schema.ConfigAndStacksInfo{
			SubCommand: "plan",
			Tags:       []string{"app"},
		}, nil)
		require.Equal(t, []string{"app"}, executed)
	})

	t.Run("include-dependents honours tags so non-matching dependents are skipped", func(t *testing.T) {
		executed := executedClosureComponents(t, &schema.ConfigAndStacksInfo{
			SubCommand:        "plan",
			Tags:              []string{"network"},
			IncludeDependents: -1,
		}, nil)
		require.Equal(t, []string{"vpc"}, executed)
	})

	t.Run("destroy with dependents only destroys matching nodes", func(t *testing.T) {
		executed := executedClosureComponents(t, &schema.ConfigAndStacksInfo{
			SubCommand:        "destroy",
			Tags:              []string{"network"},
			IncludeDependents: -1,
		}, nil)
		require.Equal(t, []string{"vpc"}, executed)
	})

	t.Run("all with include-dependents and tags equals tags alone", func(t *testing.T) {
		executed := executedClosureComponents(t, &schema.ConfigAndStacksInfo{
			SubCommand:        "plan",
			Tags:              []string{"data"},
			IncludeDependents: -1,
		}, nil)
		require.Equal(t, []string{"database"}, executed)
	})

	t.Run("non-matching seed still reaches its matching transitive dependent", func(t *testing.T) {
		executed := executedClosureComponents(t, &schema.ConfigAndStacksInfo{
			SubCommand:        "plan",
			Tags:              []string{"app"},
			IncludeDependents: -1,
		}, &TerraformSelection{NodeIDs: []string{"vpc-dev"}})
		require.Equal(t, []string{"app"}, executed)
	})

	t.Run("destroy of a non-matching seed destroys only the matching dependent", func(t *testing.T) {
		executed := executedClosureComponents(t, &schema.ConfigAndStacksInfo{
			SubCommand:        "destroy",
			Tags:              []string{"app"},
			IncludeDependents: -1,
		}, &TerraformSelection{NodeIDs: []string{"vpc-dev"}})
		require.Equal(t, []string{"app"}, executed)
	})

	t.Run("dependent depth is counted through non-matching intermediates", func(t *testing.T) {
		executed := executedClosureComponents(t, &schema.ConfigAndStacksInfo{
			SubCommand:        "plan",
			Tags:              []string{"app"},
			IncludeDependents: 1,
		}, &TerraformSelection{NodeIDs: []string{"vpc-dev"}})
		require.Empty(t, executed, "app is two hops from vpc, so depth 1 must not reach it")

		executed = executedClosureComponents(t, &schema.ConfigAndStacksInfo{
			SubCommand:        "plan",
			Tags:              []string{"app"},
			IncludeDependents: 2,
		}, &TerraformSelection{NodeIDs: []string{"vpc-dev"}})
		require.Equal(t, []string{"app"}, executed)
	})

	t.Run("destroy with dependencies destroys the seed first and prerequisites last", func(t *testing.T) {
		// Seed app and pull in its dependency closure (database, vpc); on
		// destroy the graph is reversed, so the seed is torn down before the
		// prerequisites it depends on.
		executed := executedClosureComponents(t, &schema.ConfigAndStacksInfo{
			SubCommand:          "destroy",
			Tags:                []string{"app"},
			IncludeDependencies: -1,
		}, nil)
		require.Equal(t, []string{"app", "database", "vpc"}, executed)
	})
}

// TestExecuteTerraformIncludeDependenciesCrossesStackBoundaries proves the
// dependency closure follows edges into OTHER stacks: app in stack "dev"
// depends on vpc in stack "core" (dependencies.components with an explicit
// stack), so seeding only stack "dev" with --include-dependencies must also
// execute vpc in "core" — and in dependency order.
func TestExecuteTerraformIncludeDependenciesCrossesStackBoundaries(t *testing.T) {
	t.Run("plan runs the cross-stack prerequisite first", func(t *testing.T) {
		executed := executedClosureNodes(t, terraformCrossStackClosureTestStacks(), &schema.ConfigAndStacksInfo{
			SubCommand:          "plan",
			Stack:               "dev",
			IncludeDependencies: -1,
		})
		require.Equal(t, []string{"vpc-core", "app-dev"}, executed)
	})

	t.Run("destroy reverses the cross-stack order", func(t *testing.T) {
		executed := executedClosureNodes(t, terraformCrossStackClosureTestStacks(), &schema.ConfigAndStacksInfo{
			SubCommand:          "destroy",
			Stack:               "dev",
			IncludeDependencies: -1,
		})
		require.Equal(t, []string{"app-dev", "vpc-core"}, executed)
	})

	t.Run("without closure flags the stack filter excludes the cross-stack prerequisite", func(t *testing.T) {
		executed := executedClosureNodes(t, terraformCrossStackClosureTestStacks(), &schema.ConfigAndStacksInfo{
			SubCommand: "plan",
			Stack:      "dev",
		})
		require.Equal(t, []string{"app-dev"}, executed)
	})
}

func TestExecuteTerraformClosureMergesInfoAndSelection(t *testing.T) {
	// The selection seeds app without closure; info adds unlimited
	// dependencies — the adapter must OR the two.
	executed := executedClosureComponents(t, &schema.ConfigAndStacksInfo{
		SubCommand:          "plan",
		IncludeDependencies: -1,
	}, &TerraformSelection{NodeIDs: []string{"app-dev"}})
	require.Equal(t, []string{"vpc", "database", "app"}, executed)
}

func TestExecuteTerraformClosureDoesNotReapplyQueryToPrerequisites(t *testing.T) {
	// The query selects only the app component; with closure expansion its
	// prerequisites must still run even though they fail the query — the
	// per-node dispatch skip is suppressed when the query was applied at
	// seed selection.
	executed := executedClosureComponents(t, &schema.ConfigAndStacksInfo{
		SubCommand:          "plan",
		Query:               `.metadata.tags | contains(["app"])`,
		IncludeDependencies: -1,
	}, nil)
	require.Equal(t, []string{"vpc", "database", "app"}, executed)
}

// TestTerraformClosureSpecMerging covers the depth-merge rules: most
// permissive wins per direction (unlimited beats bounded, larger bound beats
// smaller), and the flag encoding (-1 unlimited, N>0 depth) maps onto the
// filter encoding (0 unlimited).
func TestTerraformClosureSpecMerging(t *testing.T) {
	tests := []struct {
		name      string
		info      *schema.ConfigAndStacksInfo
		selection *TerraformSelection
		want      terraformClosure
	}{
		{
			name: "nil inputs disable closure",
			want: terraformClosure{},
		},
		{
			name: "info unlimited",
			info: &schema.ConfigAndStacksInfo{IncludeDependencies: -1},
			want: terraformClosure{includeDependencies: true},
		},
		{
			name: "info bounded depth",
			info: &schema.ConfigAndStacksInfo{IncludeDependents: 2},
			want: terraformClosure{includeDependents: true, dependentDepth: 2},
		},
		{
			name:      "selection unlimited beats info bounded",
			info:      &schema.ConfigAndStacksInfo{IncludeDependencies: 2},
			selection: &TerraformSelection{IncludeDependencies: true},
			want:      terraformClosure{includeDependencies: true},
		},
		{
			name:      "larger bound wins",
			info:      &schema.ConfigAndStacksInfo{IncludeDependencies: 3},
			selection: &TerraformSelection{IncludeDependencies: true, DependencyDepth: 1},
			want:      terraformClosure{includeDependencies: true, dependencyDepth: 3},
		},
		{
			// The --affected path derives both inputs from the SAME flag
			// (affectedTerraformSelection copies the parsed depth onto the
			// selection), so merging them must preserve the user's bound
			// instead of widening to unlimited.
			name:      "affected path: selection carrying the flag depth keeps the bound",
			info:      &schema.ConfigAndStacksInfo{IncludeDependents: 2},
			selection: &TerraformSelection{IncludeDependents: true, DependentDepth: 2},
			want:      terraformClosure{includeDependents: true, dependentDepth: 2},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, terraformClosureSpec(tc.info, tc.selection))
		})
	}
}

// terraformLabeledChainTestStacks builds base (ci=auto) <- middle (ci=manual) <- leaf (ci=auto)
// in stack "dev", where an arrow reads "is depended on by".
func terraformLabeledChainTestStacks() map[string]any {
	component := func(ci string, dependsOn map[string]any) map[string]any {
		section := map[string]any{
			cfg.MetadataSectionName: map[string]any{
				"component": "mock",
				"labels":    map[string]any{"ci": ci},
			},
			"vars": map[string]any{},
		}
		if dependsOn != nil {
			section[cfg.SettingsSectionName] = map[string]any{"depends_on": dependsOn}
		}
		return section
	}

	return map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"base":   component("auto", nil),
					"middle": component("manual", map[string]any{"1": map[string]any{"component": "base"}}),
					"leaf":   component("auto", map[string]any{"1": map[string]any{"component": "middle"}}),
				},
			},
		},
	}
}

// TestExecuteTerraformDependentsContractThroughDroppedIntermediates proves a
// dependent dropped by --labels still orders its surviving neighbors: the
// edge leaf -> base survives both the filter and the strict scoped rebuild.
func TestExecuteTerraformDependentsContractThroughDroppedIntermediates(t *testing.T) {
	tests := []struct {
		name       string
		subCommand string
		expected   []string
	}{
		{"plan runs base before leaf", "plan", []string{"base-dev", "leaf-dev"}},
		{"destroy runs leaf before base", "destroy", []string{"leaf-dev", "base-dev"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var executed []string
			err := ExecuteTerraform(context.Background(), TerraformOptions{
				AtmosConfig: &schema.AtmosConfiguration{},
				Info: &schema.ConfigAndStacksInfo{
					SubCommand:        tt.subCommand,
					Labels:            map[string]string{"ci": "auto"},
					IncludeDependents: -1,
				},
				Stacks:    terraformLabeledChainTestStacks(),
				Selection: &TerraformSelection{NodeIDs: []string{"base-dev"}},
				Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
					executed = append(executed, terraformNodeID(execution.Info.Component, execution.Info.Stack))
					return TerraformExecutionResult{}, nil
				},
			})
			require.NoError(t, err)
			require.Equal(t, tt.expected, executed)
		})
	}
}
