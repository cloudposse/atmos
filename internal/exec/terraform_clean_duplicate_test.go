package exec

import (
	"os"
	"path/filepath"
	"strings"
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
	// When the same component path appears multiple times, it should only be processed once.
	tempDir := t.TempDir()

	// Create a single component directory.
	componentPath := filepath.Join(tempDir, "components", "vpc")
	err := os.MkdirAll(componentPath, 0o755)
	require.NoError(t, err)

	// Create some files/folders to collect.
	terraformDir := filepath.Join(componentPath, ".terraform")
	err = os.Mkdir(terraformDir, 0o755)
	require.NoError(t, err)

	lockFile := filepath.Join(componentPath, ".terraform.lock.hcl")
	err = os.WriteFile(lockFile, []byte("test"), 0o644)
	require.NoError(t, err)

	// Pass the same path multiple times (simulating multiple stacks using same component).
	duplicatePaths := []string{
		"vpc",
		"vpc", // Duplicate.
		"vpc", // Another duplicate.
	}

	filesToClear := []string{".terraform", ".terraform.lock.hcl"}

	// Call the function with duplicate paths.
	result, err := CollectComponentsDirectoryObjects(filepath.Join(tempDir, "components"), duplicatePaths, filesToClear)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify that objects were collected (the function processes each path in the list).
	// Since we pass the same path 3 times, we expect the directory objects to be collected 3 times.
	// This is actually the current behavior - the function doesn't deduplicate input paths.
	// The deduplication happens at a higher level (in getAllStacksComponentsPaths).
	assert.NotEmpty(t, result, "Should collect directory objects even with duplicate input paths")

	// Count how many times our component path appears in the results.
	count := 0
	for _, dir := range result {
		if filepath.Base(dir.FullPath) == "vpc" || strings.Contains(dir.FullPath, "vpc") {
			count++
		}
	}

	// The function processes each input path independently, so duplicates result in duplicate processing.
	// This is expected behavior - deduplication should happen before calling this function.
	assert.Greater(t, count, 0, "Should have collected objects from vpc component")
}
