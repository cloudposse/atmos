package dependencies

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExtractComponentDependencies_ComponentsPreferredOverSettings verifies that
// `dependencies.components` wins over legacy `settings.depends_on` when both are
// present.
func TestExtractComponentDependencies_ComponentsPreferredOverSettings(t *testing.T) {
	section := map[string]any{
		"dependencies": map[string]any{
			"components": []any{map[string]any{"component": "vpc"}},
		},
		"settings": map[string]any{
			"depends_on": map[string]any{"1": map[string]any{"component": "ignored"}},
		},
	}

	deps := extractComponentDependencies(section)
	require.Len(t, deps, 1)
	assert.Equal(t, "vpc", deps[0].Component)
}

// TestExtractComponentDependencies_EmptyComponentsAuthoritative verifies that an
// explicit empty `dependencies.components: []` clears all edges and does NOT fall
// back to legacy settings.
func TestExtractComponentDependencies_EmptyComponentsAuthoritative(t *testing.T) {
	section := map[string]any{
		"dependencies": map[string]any{"components": []any{}},
		"settings": map[string]any{
			"depends_on": map[string]any{"1": map[string]any{"component": "vpc"}},
		},
	}

	assert.Empty(t, extractComponentDependencies(section))
}

// TestExtractComponentDependencies_SettingsFallback verifies the legacy path is
// used only when `dependencies.components` is entirely absent.
func TestExtractComponentDependencies_SettingsFallback(t *testing.T) {
	section := dependsOn(map[string]any{"component": "vpc", "stack": "prod"})

	deps := extractComponentDependencies(section)
	require.Len(t, deps, 1)
	assert.Equal(t, "vpc", deps[0].Component)
	assert.Equal(t, "prod", deps[0].Stack)
}

func TestExtractComponentDependencies_None(t *testing.T) {
	assert.Empty(t, extractComponentDependencies(map[string]any{}))
	assert.Empty(t, extractComponentDependencies(map[string]any{"settings": map[string]any{}}))
}

// TestExtractComponentDependencies_SettingsSkipsEmptyComponent verifies a
// settings.depends_on entry with no component is dropped.
func TestExtractComponentDependencies_SettingsSkipsEmptyComponent(t *testing.T) {
	section := map[string]any{
		"settings": map[string]any{
			"depends_on": map[string]any{
				"1": map[string]any{"component": "vpc"},
				"2": map[string]any{"stack": "prod"}, // no component → skipped
			},
		},
	}

	deps := extractComponentDependencies(section)
	require.Len(t, deps, 1)
	assert.Equal(t, "vpc", deps[0].Component)
}

// TestFilterComponentDependencies_DropsFileFolderAndEmpty verifies only true
// component-to-component edges survive.
func TestFilterComponentDependencies_DropsFileFolderAndEmpty(t *testing.T) {
	deps := []schema.ComponentDependency{
		{Component: "vpc"},
		{Kind: "file", Path: "configs/lambda.json"},
		{Kind: "folder", Path: "configs"},
		{Component: ""}, // empty component dropped
	}

	out := filterComponentDependencies(deps)
	require.Len(t, out, 1)
	assert.Equal(t, "vpc", out[0].Component)
}

func TestFilterComponentDependencies_AllDroppedReturnsNil(t *testing.T) {
	deps := []schema.ComponentDependency{
		{Kind: "file", Path: "x.json"},
	}
	assert.Nil(t, filterComponentDependencies(deps))
	assert.Nil(t, filterComponentDependencies(nil))
}

// TestNodeID_CollisionSafe verifies the length-prefixed encoding never collides
// for distinct (component, stack) pairs that share a naive delimiter split.
func TestNodeID_CollisionSafe(t *testing.T) {
	assert.NotEqual(t, NodeID("app-prod", "us"), NodeID("app", "prod-us"))
	assert.Equal(t, NodeID("app", "dev"), NodeID("app", "dev"))
	assert.NotEqual(t, NodeID("a", "b"), NodeID("b", "a"))
}
