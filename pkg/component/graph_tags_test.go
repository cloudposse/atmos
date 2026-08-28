package component

import (
	"testing"

	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// graphTagsTestStacks builds three independent (no dependency edges) Kubernetes
// components in stacks "dev"/"prod", each with distinct metadata.tags/labels, so
// tag and label filtering can be asserted precisely.
func graphTagsTestStacks() map[string]any {
	component := func(tags []any, labels map[string]any) map[string]any {
		return map[string]any{
			cfg.MetadataSectionName: map[string]any{
				"tags":   tags,
				"labels": labels,
			},
		}
	}

	return map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.KubernetesComponentType: map[string]any{
					"api": component(
						[]any{"production", "compute"},
						map[string]any{"cost-center": "platform"},
					),
					"db": component(
						[]any{"development"},
						map[string]any{"cost-center": "data"},
					),
				},
			},
		},
		"prod": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.KubernetesComponentType: map[string]any{
					"api": component(
						[]any{"production", "networking"},
						map[string]any{"cost-center": "platform", "compliance": "sox"},
					),
				},
			},
		},
	}
}

func TestFilterGraphByTagsAndLabels(t *testing.T) {
	graph, err := BuildGraph(graphTagsTestStacks(), cfg.KubernetesComponentType)
	require.NoError(t, err)
	require.Equal(t, 3, graph.Size())

	t.Run("no tags/labels set returns graph unchanged", func(t *testing.T) {
		filtered := FilterGraph(graph, &schema.ConfigAndStacksInfo{}, nil)
		require.Equal(t, 3, filtered.Size())
	})

	t.Run("tags any-match narrows across stacks", func(t *testing.T) {
		filtered := FilterGraph(graph, &schema.ConfigAndStacksInfo{Tags: []string{"production"}}, nil)
		require.Equal(t, 2, filtered.Size())
		require.Contains(t, filtered.Nodes, GraphNodeID("api", "dev"))
		require.Contains(t, filtered.Nodes, GraphNodeID("api", "prod"))
		require.NotContains(t, filtered.Nodes, GraphNodeID("db", "dev"))
	})

	t.Run("labels all-match requires every pair", func(t *testing.T) {
		filtered := FilterGraph(graph, &schema.ConfigAndStacksInfo{
			Labels: map[string]string{"cost-center": "platform", "compliance": "sox"},
		}, nil)
		require.Equal(t, 1, filtered.Size())
		require.Contains(t, filtered.Nodes, GraphNodeID("api", "prod"))
	})

	t.Run("stack filter composes with tags", func(t *testing.T) {
		filtered := FilterGraph(graph, &schema.ConfigAndStacksInfo{
			Stack: "dev",
			Tags:  []string{"production"},
		}, nil)
		require.Equal(t, 1, filtered.Size())
		require.Contains(t, filtered.Nodes, GraphNodeID("api", "dev"))
	})

	t.Run("explicit selection composes with tags (--affected --tags)", func(t *testing.T) {
		filtered := FilterGraph(graph, &schema.ConfigAndStacksInfo{Tags: []string{"production"}}, &GraphSelection{
			NodeIDs: []string{GraphNodeID("api", "dev"), GraphNodeID("db", "dev")},
		})
		require.Equal(t, 1, filtered.Size())
		require.Contains(t, filtered.Nodes, GraphNodeID("api", "dev"))
	})

	t.Run("no matches returns empty graph, not an error", func(t *testing.T) {
		filtered := FilterGraph(graph, &schema.ConfigAndStacksInfo{Tags: []string{"nonexistent"}}, nil)
		require.Equal(t, 0, filtered.Size())
	})
}

func TestMatchesGraphTagsAndLabels(t *testing.T) {
	graph, err := BuildGraph(graphTagsTestStacks(), cfg.KubernetesComponentType)
	require.NoError(t, err)
	node := graph.Nodes[GraphNodeID("api", "prod")]

	t.Run("nil node never matches", func(t *testing.T) {
		require.False(t, matchesGraphTagsAndLabels(nil, &schema.ConfigAndStacksInfo{Tags: []string{"production"}}))
	})

	t.Run("tags any-match", func(t *testing.T) {
		require.True(t, matchesGraphTagsAndLabels(node, &schema.ConfigAndStacksInfo{Tags: []string{"networking", "nonexistent"}}))
		require.False(t, matchesGraphTagsAndLabels(node, &schema.ConfigAndStacksInfo{Tags: []string{"nonexistent"}}))
	})

	t.Run("labels all-match", func(t *testing.T) {
		require.True(t, matchesGraphTagsAndLabels(node, &schema.ConfigAndStacksInfo{Labels: map[string]string{"compliance": "sox"}}))
		require.False(t, matchesGraphTagsAndLabels(node, &schema.ConfigAndStacksInfo{Labels: map[string]string{"compliance": "wrong"}}))
	})

	t.Run("tags and labels together", func(t *testing.T) {
		require.True(t, matchesGraphTagsAndLabels(node, &schema.ConfigAndStacksInfo{
			Tags:   []string{"production"},
			Labels: map[string]string{"compliance": "sox"},
		}))
		require.False(t, matchesGraphTagsAndLabels(node, &schema.ConfigAndStacksInfo{
			Tags:   []string{"production"},
			Labels: map[string]string{"compliance": "wrong"},
		}))
	})
}
