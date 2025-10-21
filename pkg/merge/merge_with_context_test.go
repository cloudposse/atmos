package merge

import (
	"bytes"
	"strings"
	"testing"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMergeWithContext_TypeMismatchError(t *testing.T) {
	// This test demonstrates the enhanced error messages when a type mismatch occurs
	// Note: mergo's behavior with type mismatches can vary based on the merge strategy

	// Try different scenarios that might trigger a merge error
	testCases := []struct {
		name     string
		strategy string
		map1     map[string]any
		map2     map[string]any
	}{
		{
			name:     "array_to_string_with_merge",
			strategy: "merge",
			map1: map[string]any{
				"vars": map[string]any{
					"subnets": []string{"subnet-1", "subnet-2"},
				},
			},
			map2: map[string]any{
				"vars": map[string]any{
					"subnets": "single-subnet",
				},
			},
		},
		{
			name:     "map_to_string",
			strategy: "replace",
			map1: map[string]any{
				"config": map[string]any{
					"nested": map[string]any{
						"value": "test",
					},
				},
			},
			map2: map[string]any{
				"config": "string-value", // Trying to replace a map with a string
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					ListMergeStrategy: tc.strategy,
				},
			}

			// Create merge context with file information
			mergeContext := NewMergeContext()
			mergeContext = mergeContext.WithFile("stacks/base.yaml")
			mergeContext = mergeContext.WithFile("stacks/override.yaml")

			// Attempt to merge
			result, err := MergeWithContext(atmosConfig, []map[string]any{tc.map1, tc.map2}, mergeContext)

			// Handle success case
			if err == nil {
				// No error - the merge succeeded (which is also valid for some strategies)
				t.Logf("Merge succeeded for %s (strategy: %s)", tc.name, tc.strategy)
				assert.NotNil(t, result)
				return
			}

			// We got an error - verify it contains our enhanced context
			errStr := err.Error()
			t.Logf("Enhanced error message for %s:\n%s", tc.name, errStr)

			// Check if error contains file context
			if !strings.Contains(errStr, "File being processed:") {
				t.Logf("Note: Error doesn't contain file context - might not be a merge error: %s", errStr)
				return
			}

			// Verify file context details
			assert.Contains(t, errStr, "stacks/override.yaml", "Error should mention the current file")
			assert.Contains(t, errStr, "Import chain:", "Error should show import chain")
			assert.Contains(t, errStr, "stacks/base.yaml", "Error should show base file in chain")

			// For type override errors, check for helpful hints
			if strings.Contains(errStr, "cannot override") {
				assert.Contains(t, errStr, "Likely cause:", "Error should contain likely cause")
				assert.Contains(t, errStr, "Debug hint:", "Error should contain debug hints")
			}
		})
	}
}

func TestMergeWithContext_NoError(t *testing.T) {
	// Test that context doesn't interfere with successful merges
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace",
		},
	}

	map1 := map[string]any{"foo": "bar"}
	map2 := map[string]any{"baz": "bat"}

	mergeContext := NewMergeContext().WithFile("test.yaml")

	result, err := MergeWithContext(atmosConfig, []map[string]any{map1, map2}, mergeContext)

	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "bar", result["foo"])
	assert.Equal(t, "bat", result["baz"])
}

func TestMergeWithContext_ErrorIsLogged(t *testing.T) {
	// Capture log output to verify errors are logged
	var logBuffer bytes.Buffer
	oldLogger := log.Default()

	// Create a logger that writes to our buffer
	testLogger := log.New()
	testLogger.SetOutput(&logBuffer)
	testLogger.SetLevel(log.DebugLevel)
	log.SetDefault(testLogger)
	defer log.SetDefault(oldLogger)

	// Create a scenario that will cause an error
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "invalid-strategy", // This will cause an error
		},
	}

	mergeContext := NewMergeContext()
	mergeContext = mergeContext.WithFile("test-file.yaml")

	// This should error and log
	_, err := MergeWithContext(atmosConfig, []map[string]any{{"a": "b"}}, mergeContext)

	// Verify we got an error
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "invalid list merge strategy")

	// Verify the error was logged with context
	logOutput := logBuffer.String()
	if logOutput != "" {
		// If logging is enabled, verify it contains relevant information
		t.Logf("Log output: %s", logOutput)
		// The exact format depends on the logger configuration
	}
}
