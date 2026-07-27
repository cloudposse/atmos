package list

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/list/dependencies"
)

// closurePreviewStacksMap builds a described-stacks map with a cross-stack
// dependency chain: dev/app -> dev/db -> core/vpc, plus an unrelated stack.
func closurePreviewStacksMap() map[string]any {
	component := func(dependsOn map[string]any) map[string]any {
		section := map[string]any{
			"metadata": map[string]any{},
			"vars":     map[string]any{},
		}
		if dependsOn != nil {
			section["settings"] = map[string]any{"depends_on": dependsOn}
		}
		return section
	}
	return map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"app": component(map[string]any{"1": map[string]any{"component": "db"}}),
					"db":  component(map[string]any{"1": map[string]any{"component": "vpc", "stack": "core"}}),
				},
			},
		},
		"core": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": component(nil),
				},
			},
		},
		"unrelated": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"poison": component(nil),
				},
			},
		},
	}
}

// TestStackRowsFromClosure verifies the closure's stacks shape into sorted rows.
func TestStackRowsFromClosure(t *testing.T) {
	t.Parallel()

	graph, err := dependencies.BuildGraph(closurePreviewStacksMap())
	require.NoError(t, err)
	roots := dependencies.Roots(graph, &dependencies.Selector{Components: []string{"app"}})
	closure := dependencies.ReachableClosure(graph, roots, dependencies.DirectionForward, dependencies.Depths{})

	rows := stackRowsFromClosure(closure)
	var names []string
	for _, row := range rows {
		names = append(names, row["stack"].(string))
	}
	assert.Equal(t, []string{"core", "dev"}, names)
}

// TestFilterRowsByComponentNames verifies the membership row gate.
func TestFilterRowsByComponentNames(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"component": "app"},
		{"component": "db"},
		{"component": "poison"},
	}
	filtered := filterRowsByComponentNames(rows, []string{"app", "db"})
	require.Len(t, filtered, 2)
	assert.Equal(t, "app", filtered[0]["component"])
	assert.Equal(t, "db", filtered[1]["component"])
}
