package adapters

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
)

// terraformTagsTestStacks builds three independent (no dependency edges)
// components in stack "dev", each with distinct metadata.tags/labels, so tag
// and label filtering can be asserted precisely without dependency-order noise.
func terraformTagsTestStacks() map[string]any {
	component := func(tags []any, labels map[string]any) map[string]any {
		return map[string]any{
			cfg.MetadataSectionName: map[string]any{
				"component": "mock",
				"tags":      tags,
				"labels":    labels,
			},
			"vars": map[string]any{},
		}
	}

	return map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"vpc": component(
						[]any{"production", "networking"},
						map[string]any{"cost-center": "platform", "compliance": "sox"},
					),
					"eks": component(
						[]any{"production", "compute"},
						map[string]any{"cost-center": "platform"},
					),
					"rds": component(
						[]any{"development"},
						map[string]any{"cost-center": "data"},
					),
				},
			},
		},
	}
}

func executedComponents(ctx context.Context, t *testing.T, info *schema.ConfigAndStacksInfo) []string {
	t.Helper()

	var executed []string
	err := ExecuteTerraform(ctx, TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info:        info,
		Stacks:      terraformTagsTestStacks(),
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			executed = append(executed, execution.Info.Component)
			return TerraformExecutionResult{}, nil
		},
	})
	require.NoError(t, err)
	sort.Strings(executed)
	return executed
}

func TestExecuteTerraformFiltersByTags(t *testing.T) {
	ctx := context.Background()

	t.Run("--tags alone selects matching components across the whole run", func(t *testing.T) {
		executed := executedComponents(ctx, t, &schema.ConfigAndStacksInfo{
			Tags:       []string{"production"},
			SubCommand: "plan",
		})
		require.Equal(t, []string{"eks", "vpc"}, executed)
	})

	t.Run("--all --tags composes: tags narrow the --all selection", func(t *testing.T) {
		executed := executedComponents(ctx, t, &schema.ConfigAndStacksInfo{
			All:        true,
			Tags:       []string{"development"},
			SubCommand: "plan",
		})
		require.Equal(t, []string{"rds"}, executed)
	})

	t.Run("no matching tags runs zero components, not an error", func(t *testing.T) {
		executed := executedComponents(ctx, t, &schema.ConfigAndStacksInfo{
			Tags:       []string{"nonexistent"},
			SubCommand: "plan",
		})
		require.Empty(t, executed)
	})
}

func TestExecuteTerraformFiltersByLabels(t *testing.T) {
	ctx := context.Background()

	t.Run("--labels requires all pairs to match (AND)", func(t *testing.T) {
		executed := executedComponents(ctx, t, &schema.ConfigAndStacksInfo{
			Labels:     map[string]string{"cost-center": "platform", "compliance": "sox"},
			SubCommand: "plan",
		})
		require.Equal(t, []string{"vpc"}, executed)
	})

	t.Run("single label pair matches multiple components", func(t *testing.T) {
		executed := executedComponents(ctx, t, &schema.ConfigAndStacksInfo{
			Labels:     map[string]string{"cost-center": "platform"},
			SubCommand: "plan",
		})
		require.Equal(t, []string{"eks", "vpc"}, executed)
	})

	t.Run("tags and labels compose together (both must match)", func(t *testing.T) {
		executed := executedComponents(ctx, t, &schema.ConfigAndStacksInfo{
			Tags:       []string{"production"},
			Labels:     map[string]string{"compliance": "sox"},
			SubCommand: "plan",
		})
		require.Equal(t, []string{"vpc"}, executed)
	})
}

func TestMatchesTerraformTagsAndLabels(t *testing.T) {
	node := &dependency.Node{
		Metadata: map[string]any{
			cfg.MetadataSectionName: map[string]any{
				"tags":   []any{"production", "networking"},
				"labels": map[string]any{"cost-center": "platform"},
			},
		},
	}

	t.Run("nil node never matches", func(t *testing.T) {
		require.False(t, matchesTerraformTagsAndLabels(nil, &schema.ConfigAndStacksInfo{Tags: []string{"production"}}))
	})

	t.Run("no filter set matches everything", func(t *testing.T) {
		require.True(t, matchesTerraformTagsAndLabels(node, &schema.ConfigAndStacksInfo{}))
	})

	t.Run("tags any-match", func(t *testing.T) {
		require.True(t, matchesTerraformTagsAndLabels(node, &schema.ConfigAndStacksInfo{Tags: []string{"networking", "nonexistent"}}))
		require.False(t, matchesTerraformTagsAndLabels(node, &schema.ConfigAndStacksInfo{Tags: []string{"nonexistent"}}))
	})

	t.Run("labels all-match", func(t *testing.T) {
		require.True(t, matchesTerraformTagsAndLabels(node, &schema.ConfigAndStacksInfo{Labels: map[string]string{"cost-center": "platform"}}))
		require.False(t, matchesTerraformTagsAndLabels(node, &schema.ConfigAndStacksInfo{Labels: map[string]string{"cost-center": "wrong"}}))
	})
}
