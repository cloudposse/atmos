package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAllStacksComponentsPaths_DuplicateComponents(t *testing.T) {
	// Test case: Multiple stacks referencing the same component should not produce duplicate paths.
	// Define expected unique components upfront.
	expectedComponents := map[string]bool{
		"vnet-elements": true,
		"database":      true,
		"app":           true,
	}

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
	require.NotEmpty(t, paths, "Should return at least one path")

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

	// Verify we have the correct number of unique paths.
	assert.Equal(t, len(expectedComponents), len(pathCounts),
		"Should have exactly %d unique component paths, but got %d",
		len(expectedComponents), len(pathCounts))

	// Verify all expected components are present in the result.
	for expectedComponent := range expectedComponents {
		found := false
		for path := range pathCounts {
			if path == expectedComponent {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected component '%s' not found in paths", expectedComponent)
	}

	// Also verify no unexpected components were added.
	for path := range pathCounts {
		assert.True(t, expectedComponents[path],
			"Unexpected component '%s' found in paths", path)
	}
}

func TestCollectComponentsDirectoryObjects_WithDuplicatePaths(t *testing.T) {
	// Test that CollectComponentsDirectoryObjects handles duplicate paths correctly.
	// This test would need actual file system setup or mocking, so keeping it as a placeholder.
	t.Skipf("Skipping integration test that requires file system setup")
}
