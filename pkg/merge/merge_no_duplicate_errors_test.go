package merge

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestNoDuplicateErrorPrinting verifies that errors are not printed directly to stderr.
// This was the issue where merge.go was printing errors before returning them..
func TestNoDuplicateErrorPrinting(t *testing.T) {
	// Capture stderr to detect any direct printing
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Channel to capture stderr output
	stderrChan := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		stderrChan <- buf.String()
	}()

	// Create a scenario that would trigger a validation error (invalid strategy).
	// We verify that if an error occurs, it's not printed directly to stderr
	// but returned to the caller.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "invalid-strategy", // This will cause an error
		},
	}

	map1 := map[string]any{"test": "value"}
	map2 := map[string]any{"test2": "value2"}

	// This should return an error but not print to stderr
	_, err := Merge(atmosConfig, []map[string]any{map1, map2})

	// Restore stderr
	w.Close()
	os.Stderr = oldStderr
	stderrOutput := <-stderrChan

	// Verify we got an error (invalid strategy)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "invalid list merge strategy")

	// Verify nothing was printed to stderr
	// The error should only be returned, not printed
	assert.Empty(t, stderrOutput, "No output should be printed to stderr directly from merge.go")
}

// TestMergeTypeOverride_SucceedsWithoutPrinting verifies that type overrides
// (e.g., list→scalar) succeed silently — no error returned, nothing printed.
// This is the WithOverride contract: src always wins regardless of type.
func TestMergeTypeOverride_SucceedsWithoutPrinting(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace",
		},
	}

	// src overrides a []any with a scalar string — this must succeed (WithOverride).
	map1 := map[string]any{"subnets": []any{"10.0.1.0/24"}}
	map2 := map[string]any{"subnets": "not-a-slice"}

	// Capture any direct prints to stderr.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	stderrChan := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		stderrChan <- buf.String()
	}()

	// Perform the merge — must succeed.
	result, err := Merge(atmosConfig, []map[string]any{map1, map2})

	// Close and restore stderr.
	w.Close()
	os.Stderr = oldStderr
	stderrOutput := <-stderrChan

	// Type overrides are allowed: src scalar replaces dst slice.
	assert.NoError(t, err, "type override (list→scalar) must not error")
	assert.Equal(t, "not-a-slice", result["subnets"],
		"src scalar must override dst slice")

	// Nothing should be printed to stderr.
	assert.Empty(t, stderrOutput, "Merge should not print to stderr")
}

// TestErrorContextWithoutDuplicatePrinting verifies that MergeContext
// enhances errors without causing duplicate printing..
func TestErrorContextWithoutDuplicatePrinting(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "invalid-to-cause-error",
		},
	}

	mergeContext := NewMergeContext()
	mergeContext = mergeContext.WithFile("test.yaml")

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	stderrChan := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		stderrChan <- buf.String()
	}()

	// This should error due to invalid strategy
	_, err := MergeWithContext(atmosConfig, []map[string]any{{"a": "b"}}, mergeContext)

	w.Close()
	os.Stderr = oldStderr
	stderrOutput := <-stderrChan

	// We should get an error
	assert.NotNil(t, err)

	// The error should be enhanced with context
	errStr := err.Error()
	assert.Contains(t, errStr, "File being processed: test.yaml")

	// But nothing should be printed to stderr
	assert.Empty(t, stderrOutput, "MergeWithContext should not print to stderr")
}

// TestMultipleMergeCallsNoDuplicates simulates what was happening in validate stacks
// where multiple merge errors would print multiple times..
func TestMultipleMergeCallsNoDuplicates(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace",
		},
	}

	// Capture stderr for the entire test
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	stderrChan := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		stderrChan <- buf.String()
	}()

	// Simulate multiple merge operations like in stack processing
	var errors []error
	for i := 0; i < 5; i++ {
		map1 := map[string]any{
			"key": "value",
		}
		map2 := map[string]any{
			"key": "different",
		}

		// Each merge call should not print
		result, err := Merge(atmosConfig, []map[string]any{map1, map2})
		if err != nil {
			errors = append(errors, err)
		} else {
			assert.NotNil(t, result)
		}
	}

	// Close and check stderr
	w.Close()
	os.Stderr = oldStderr
	stderrOutput := <-stderrChan

	// Even after multiple merge operations, nothing should be printed
	assert.Empty(t, stderrOutput, "Multiple merges should not print to stderr")

	// If we want to report errors, we should do it at the application level
	// not in the merge function itself
	if len(errors) > 0 {
		// This is where the application would handle/log errors appropriately
		t.Logf("Got %d errors (as expected for testing)", len(errors))
	}
}
