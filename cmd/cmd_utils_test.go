package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterStackNames(t *testing.T) {
	tests := []struct {
		name           string
		stacksBasePath string
		stacksMap      map[string]any
		expectedStacks []string
	}{
		{
			name:           "filters out paths with configured base path",
			stacksBasePath: "stacks",
			stacksMap: map[string]any{
				"stacks/test1.yaml": nil,
				"test1":             nil,
				"test2":             nil,
				"stacks/test2":      nil,
			},
			expectedStacks: []string{"test1", "test2"},
		},
		{
			name:           "handles custom base path",
			stacksBasePath: "custom/path",
			stacksMap: map[string]any{
				"custom/path/test1.yaml": nil,
				"test1":                  nil,
				"test2":                  nil,
				"custom/path/test2":      nil,
			},
			expectedStacks: []string{"test1", "test2"},
		},
		{
			name:           "handles empty base path",
			stacksBasePath: "",
			stacksMap: map[string]any{
				"test1.yaml": nil,
				"test1":      nil,
				"test2":      nil,
			},
			expectedStacks: []string{"test1", "test2", "test1.yaml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run the filter function
			stacks := filterStackNames(tt.stacksMap, tt.stacksBasePath)

			// Verify the results
			assert.ElementsMatch(t, tt.expectedStacks, stacks)
		})
	}
}
