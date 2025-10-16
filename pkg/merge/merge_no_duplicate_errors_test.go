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

	// Create a scenario that would trigger an error in mergo
	// Note: With current mergo behavior, type mismatches don't always error,
	// but we're testing that IF an error occurs, it's not printed directly
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

// TestMergeErrorsAreWrappedNotPrinted ensures that when mergo returns an error,
// we wrap it and return it rather than printing it..
func TestMergeErrorsAreWrappedNotPrinted(t *testing.T) {
	// This test verifies that our code properly wraps errors
	// without printing them directly

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace",
		},
	}

	// Create maps that are valid (mergo won't error on these)
	map1 := map[string]any{
		"config": map[string]any{
			"value": []string{"a", "b"},
		},
	}
	map2 := map[string]any{
		"config": map[string]any{
			"value": "string", // Different type, but mergo will just replace
		},
	}

	// Capture any direct prints to stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	stderrChan := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		stderrChan <- buf.String()
	}()

	// Perform the merge
	result, err := Merge(atmosConfig, []map[string]any{map1, map2})

	// Close and restore stderr
	w.Close()
	os.Stderr = oldStderr
	stderrOutput := <-stderrChan

	// With replace strategy, this should succeed (no error)
	assert.Nil(t, err)
	assert.NotNil(t, result)

	// Verify nothing was printed to stderr
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
