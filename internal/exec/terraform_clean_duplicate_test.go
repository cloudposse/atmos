package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAllStacksComponentsPaths_DuplicateComponents(t *testing.T) {
	// Test case: Multiple stacks referencing the same component should not produce duplicate paths.
	stacksMap := map[string]any{
		"stack1": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vnet-elements": map[string]any{
						"component": "vnet-elements",
					},
					"database": map[string]any{
						"component": "database",
					},
				},
			},
		},
		"stack2": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vnet-elements": map[string]any{
						"component": "vnet-elements", // Same component as stack1.
					},
					"app": map[string]any{
						"component": "app",
					},
				},
			},
		},
		"stack3": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vnet-elements": map[string]any{
						"component": "vnet-elements", // Same component again.
					},
				},
			},
		},
	}

	paths := getAllStacksComponentsPaths(stacksMap)
	require.NotEmpty(t, paths)

	// Count occurrences of each path.
	pathCounts := make(map[string]int)
	for _, path := range paths {
		pathCounts[path]++
	}

	// Check for duplicates.
	hasDuplicates := false
	for path, count := range pathCounts {
		if count > 1 {
			t.Logf("Component path '%s' appears %d times (duplicate)", path, count)
			hasDuplicates = true
		}
	}

	// After the fix, there should be no duplicates.
	assert.False(t, hasDuplicates, "Component paths should not contain duplicates")
}

func TestCollectComponentsDirectoryObjects_WithDuplicatePaths(t *testing.T) {
	// Test that CollectComponentsDirectoryObjects handles duplicate paths correctly.
	// This test would need actual file system setup or mocking, so keeping it as a placeholder.
	t.Skipf("Skipping integration test that requires file system setup")
}
