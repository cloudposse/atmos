package list

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// closurePreviewStacksMap builds a described-stacks map with a cross-tag,
// cross-stack dependency chain: dev/app (tags: app) -> dev/db (tags: data)
// -> core/vpc (tags: network), plus an unrelated stack that must never appear
// in a bounded closure preview.
func closurePreviewStacksMap() map[string]any {
	component := func(tags []any, dependsOn map[string]any) map[string]any {
		section := map[string]any{
			"metadata": map[string]any{"tags": tags},
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
					"app": component([]any{"app"}, map[string]any{"1": map[string]any{"component": "db"}}),
					"db":  component([]any{"data"}, map[string]any{"1": map[string]any{"component": "vpc", "stack": "core"}}),
				},
			},
		},
		"core": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": component([]any{"network"}, nil),
				},
			},
		},
		"unrelated": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"poison": component([]any{"other"}, nil),
				},
			},
		},
	}
}

// TestExtractStacksInClosure verifies `list stacks --include-dependencies`
// lists exactly the stacks the closure touches, including stacks whose only
// members are non-matching prerequisites.
func TestExtractStacksInClosure(t *testing.T) {
	t.Parallel()

	stackNames := func(stacks []map[string]any) []string {
		var names []string
		for _, stack := range stacks {
			names = append(names, stack["stack"].(string))
		}
		return names
	}

	t.Run("tags seed spans prerequisite stacks", func(t *testing.T) {
		t.Parallel()
		opts := &StacksOptions{Tags: []string{"app"}, IncludeDependencies: -1}
		stacks, err := extractStacksInClosure(&schema.AtmosConfiguration{}, opts, nil, closurePreviewStacksMap())
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"core", "dev"}, stackNames(stacks))
	})

	t.Run("depth 1 stays within the seed stack", func(t *testing.T) {
		t.Parallel()
		opts := &StacksOptions{Tags: []string{"app"}, IncludeDependencies: 1}
		stacks, err := extractStacksInClosure(&schema.AtmosConfiguration{}, opts, nil, closurePreviewStacksMap())
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"dev"}, stackNames(stacks))
	})

	t.Run("component seed with dependents", func(t *testing.T) {
		t.Parallel()
		opts := &StacksOptions{Component: "vpc", IncludeDependents: -1}
		stacks, err := extractStacksInClosure(&schema.AtmosConfiguration{}, opts, nil, closurePreviewStacksMap())
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"core", "dev"}, stackNames(stacks))
	})
}

// TestFilterComponentsByClosure verifies `list components --include-dependencies`
// keeps exactly the closure's components, including prerequisites that do not
// match the seeding selectors.
func TestFilterComponentsByClosure(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"component": "app"},
		{"component": "db"},
		{"component": "vpc"},
		{"component": "poison"},
	}
	componentNames := func(rows []map[string]any) []string {
		var names []string
		for _, row := range rows {
			names = append(names, row["component"].(string))
		}
		return names
	}

	t.Run("tags seed keeps non-matching prerequisites", func(t *testing.T) {
		t.Parallel()
		opts := &ComponentsOptions{Tags: []string{"app"}, IncludeDependencies: -1}
		filtered, err := filterComponentsByClosure(&schema.AtmosConfiguration{}, opts, nil, closurePreviewStacksMap(), rows)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"app", "db", "vpc"}, componentNames(filtered))
	})

	t.Run("stack glob seed", func(t *testing.T) {
		t.Parallel()
		opts := &ComponentsOptions{Stack: "de*", IncludeDependencies: -1}
		filtered, err := filterComponentsByClosure(&schema.AtmosConfiguration{}, opts, nil, closurePreviewStacksMap(), rows)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"app", "db", "vpc"}, componentNames(filtered))
	})
}
