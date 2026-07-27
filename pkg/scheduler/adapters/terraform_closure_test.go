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

	t.Run("include-dependents expands in the reverse direction", func(t *testing.T) {
		executed := executedClosureComponents(t, &schema.ConfigAndStacksInfo{
			SubCommand:        "plan",
			Tags:              []string{"network"},
			IncludeDependents: -1,
		}, nil)
		require.Equal(t, []string{"vpc", "database", "app"}, executed)
	})

	t.Run("destroy with dependents runs dependents first", func(t *testing.T) {
		executed := executedClosureComponents(t, &schema.ConfigAndStacksInfo{
			SubCommand:        "destroy",
			Tags:              []string{"network"},
			IncludeDependents: -1,
		}, nil)
		require.Equal(t, []string{"app", "database", "vpc"}, executed)
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, terraformClosureSpec(tc.info, tc.selection))
		})
	}
}
