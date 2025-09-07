package stream

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/stretchr/testify/assert"
)

// TestNoTestsDetection verifies that "No tests" is displayed for packages without test files
func TestNoTestsDetection(t *testing.T) {
	tests := []struct {
		name           string
		events         []types.TestEvent
		expectedOutput []string // Lines that should appear in output
		notExpected    []string // Lines that should NOT appear
		description    string
	}{
		{
			name: "package_with_skip_event",
			events: []types.TestEvent{
				{Action: "start", Package: "example.com/pkg1"},
				{Action: "output", Package: "example.com/pkg1", Output: "?   \texample.com/pkg1\t[no test files]\n"},
				{Action: "skip", Package: "example.com/pkg1", Elapsed: 0},
			},
			expectedOutput: []string{
				"▶ example.com/pkg1",
				"No tests",
			},
			description: "Package that emits skip event should show 'No tests'",
		},
		{
			name: "package_with_no_test_files_and_pass",
			events: []types.TestEvent{
				{Action: "start", Package: "example.com/pkg2"},
				{Action: "output", Package: "example.com/pkg2", Output: "?   \texample.com/pkg2\t[no test files]\n"},
				{Action: "pass", Package: "example.com/pkg2", Elapsed: 0}, // Happens with coverprofile
			},
			expectedOutput: []string{
				"▶ example.com/pkg2",
				"No tests",
			},
			description: "Package with [no test files] and pass event (coverprofile mode) should show 'No tests'",
		},
		{
			name: "package_with_actual_test",
			events: []types.TestEvent{
				{Action: "start", Package: "example.com/pkg3"},
				{Action: "run", Package: "example.com/pkg3", Test: "TestSomething"},
				{Action: "output", Package: "example.com/pkg3", Test: "TestSomething", Output: "=== RUN   TestSomething\n"},
				{Action: "pass", Package: "example.com/pkg3", Test: "TestSomething", Elapsed: 0.1},
				{Action: "pass", Package: "example.com/pkg3", Elapsed: 0.1},
			},
			expectedOutput: []string{
				"▶ example.com/pkg3",
				"✔ TestSomething",
			},
			notExpected: []string{
				"No tests",
			},
			description: "Package with actual test should NOT show 'No tests'",
		},
		{
			name: "multiple_packages_mixed",
			events: []types.TestEvent{
				// Package 1: has no tests
				{Action: "start", Package: "example.com/empty1"},
				{Action: "output", Package: "example.com/empty1", Output: "?   \texample.com/empty1\t[no test files]\n"},
				{Action: "skip", Package: "example.com/empty1"},
				// Package 2: has a test
				{Action: "start", Package: "example.com/withtests"},
				{Action: "run", Package: "example.com/withtests", Test: "TestExample"},
				{Action: "pass", Package: "example.com/withtests", Test: "TestExample", Elapsed: 0.01},
				{Action: "pass", Package: "example.com/withtests", Elapsed: 0.01},
				// Package 3: has no tests (coverprofile mode)
				{Action: "start", Package: "example.com/empty2"},
				{Action: "output", Package: "example.com/empty2", Output: "?   \texample.com/empty2\t[no test files]\n"},
				{Action: "pass", Package: "example.com/empty2", Elapsed: 0},
			},
			expectedOutput: []string{
				"▶ example.com/empty1",
				"No tests",
				"▶ example.com/withtests",
				"✔ TestExample",
				"▶ example.com/empty2",
				"No tests",
			},
			description: "Multiple packages should each show correct status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock JSON stream from events
			var jsonStream bytes.Buffer
			for _, event := range tt.events {
				data, err := json.Marshal(event)
				assert.NoError(t, err)
				jsonStream.Write(data)
				jsonStream.WriteByte('\n')
			}

			// Capture output
			output := captureStreamProcessorOutput(&jsonStream)

			// Debug: print the actual output
			t.Logf("Test: %s\nOutput: %q", tt.name, output)

			// Check expected output
			for _, expected := range tt.expectedOutput {
				assert.Contains(t, output, expected,
					fmt.Sprintf("%s: Expected to find '%s' in output\nActual output: %s", tt.description, expected, output))
			}

			// Check not expected output
			for _, notExpected := range tt.notExpected {
				assert.NotContains(t, output, notExpected,
					fmt.Sprintf("%s: Should NOT find '%s' in output", tt.description, notExpected))
			}
		})
	}
}

// captureStreamProcessorOutput runs the StreamProcessor and captures its output
func captureStreamProcessorOutput(jsonStream io.Reader) string {
	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Create a temp JSON file for output
	tmpFile, err := os.CreateTemp("", "test-output-*.json")
	if err != nil {
		panic(err)
	}
	defer os.Remove(tmpFile.Name())

	// Create processor
	processor := NewStreamProcessor(tmpFile, "all", "", "standard")

	// Process the stream
	processor.ProcessStream(jsonStream)

	// Close writer and restore stderr
	w.Close()
	output, _ := io.ReadAll(r)
	os.Stderr = oldStderr

	tmpFile.Close()

	return string(output)
}
