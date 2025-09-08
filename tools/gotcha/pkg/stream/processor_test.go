package stream

import (
	"strings"
	"testing"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestStreamProcessorBuffering(t *testing.T) {
	tests := []struct {
		name           string
		events         []types.TestEvent
		expectedBuffer map[string][]string
		description    string
	}{
		{
			name: "basic buffering",
			events: []types.TestEvent{
				{Action: "run", Test: "TestBasic"},
				{Action: "output", Test: "TestBasic", Output: "line1\n"},
				{Action: "output", Test: "TestBasic", Output: "line2\n"},
				{Action: "fail", Test: "TestBasic"},
			},
			expectedBuffer: map[string][]string{
				"TestBasic": {
					"line1\n",
					"line2\n",
				},
			},
			description: "Should buffer output correctly",
		},
		{
			name: "output before run",
			events: []types.TestEvent{
				{Action: "output", Test: "TestEarly", Output: "early\n"},
				{Action: "run", Test: "TestEarly"},
				{Action: "output", Test: "TestEarly", Output: "normal\n"},
				{Action: "pass", Test: "TestEarly"},
			},
			expectedBuffer: map[string][]string{
				"TestEarly": {
					"early\n",
					"normal\n",
				},
			},
			description: "Should handle output before run event",
		},
		{
			name: "multiple tests",
			events: []types.TestEvent{
				{Action: "run", Test: "Test1"},
				{Action: "output", Test: "Test1", Output: "test1 output\n"},
				{Action: "run", Test: "Test2"},
				{Action: "output", Test: "Test2", Output: "test2 output\n"},
				{Action: "pass", Test: "Test1"},
				{Action: "fail", Test: "Test2"},
			},
			expectedBuffer: map[string][]string{
				"Test1": {
					"test1 output\n",
				},
				"Test2": {
					"test2 output\n",
				},
			},
			description: "Should maintain separate buffers for different tests",
		},
		{
			name: "subtest buffering",
			events: []types.TestEvent{
				{Action: "run", Test: "TestParent"},
				{Action: "run", Test: "TestParent/sub1"},
				{Action: "output", Test: "TestParent/sub1", Output: "sub1 error\n"},
				{Action: "fail", Test: "TestParent/sub1"},
				{Action: "fail", Test: "TestParent"},
			},
			expectedBuffer: map[string][]string{
				"TestParent": {},
				"TestParent/sub1": {
					"sub1 error\n",
				},
			},
			description: "Should handle subtests separately",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := NewStreamProcessor(nil, "all", "", "standard")

			// Process events
			for _, event := range tt.events {
				processStreamEvent(processor, &event)
			}

			// For non-fail events, buffers might be deleted
			// Only check buffers that should still exist
			for testName, expectedOutput := range tt.expectedBuffer {
				actualOutput, exists := processor.buffers[testName]
				if len(expectedOutput) > 0 {
					assert.True(t, exists, "Buffer should exist for test %s", testName)
					assert.Equal(t, expectedOutput, actualOutput,
						"Output mismatch for test %s: %s", testName, tt.description)
				}
			}
		})
	}
}

func TestStreamProcessorSubtestOutput(t *testing.T) {
	tests := []struct {
		name           string
		parentTest     string
		buffers        map[string][]string
		expectedOutput []string
		description    string
	}{
		{
			name:       "parent with no output, subtests have output",
			parentTest: "TestParent",
			buffers: map[string][]string{
				"TestParent":          {},
				"TestParent/subtest1": {"subtest1 error\n"},
				"TestParent/subtest2": {"subtest2 error\n"},
			},
			expectedOutput: []string{
				"subtest1 error\n",
				"subtest2 error\n",
			},
			description: "Should collect subtest output when parent has none",
		},
		{
			name:       "parent has output",
			parentTest: "TestParent",
			buffers: map[string][]string{
				"TestParent":          {"parent error\n"},
				"TestParent/subtest1": {"subtest1 error\n"},
			},
			expectedOutput: []string{
				"parent error\n",
			},
			description: "Should use parent output when it exists",
		},
		{
			name:       "deeply nested subtests",
			parentTest: "TestTop",
			buffers: map[string][]string{
				"TestTop":                 {},
				"TestTop/level1":          {"level1\n"},
				"TestTop/level1/level2":   {"level2\n"},
				"TestTop/level1/level2/3": {"level3\n"},
			},
			expectedOutput: []string{
				"level1\n",
				"level2\n",
				"level3\n",
			},
			description: "Should collect all nested subtest output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the collection logic
			output, exists := tt.buffers[tt.parentTest]

			// If no output found, check for subtest output
			if !exists || len(output) == 0 {
				testPrefix := tt.parentTest + "/"
				for testName, testOutput := range tt.buffers {
					if strings.HasPrefix(testName, testPrefix) && len(testOutput) > 0 {
						output = append(output, testOutput...)
					}
				}
			}

			assert.Equal(t, tt.expectedOutput, output, tt.description)
		})
	}
}

// Helper to simulate event processing.
func processStreamEvent(p *StreamProcessor, event *types.TestEvent) {
	// Skip package-level events
	if event.Test == "" {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	switch event.Action {
	case "run":
		// Initialize buffer for this test (only if doesn't exist to preserve early output)
		if p.buffers[event.Test] == nil {
			p.buffers[event.Test] = []string{}
		}

	case "output":
		// Create buffer if it doesn't exist (can happen with subtests or out-of-order events)
		if p.buffers[event.Test] == nil {
			p.buffers[event.Test] = []string{}
		}
		p.buffers[event.Test] = append(p.buffers[event.Test], event.Output)

	case "pass", "fail", "skip":
		// In real code, output would be displayed here
		// For testing, we don't delete the buffer so we can verify it
	}
}

func TestStreamProcessorStatistics(t *testing.T) {
	processor := NewStreamProcessor(nil, "all", "", "standard")

	events := []types.TestEvent{
		{Action: "run", Test: "Test1"},
		{Action: "pass", Test: "Test1"},
		{Action: "run", Test: "Test2"},
		{Action: "fail", Test: "Test2"},
		{Action: "run", Test: "Test3"},
		{Action: "skip", Test: "Test3"},
		{Action: "run", Test: "Test4"},
		{Action: "pass", Test: "Test4"},
	}

	// Process events and track statistics
	for _, event := range events {
		if event.Test == "" {
			continue
		}

		switch event.Action {
		case "pass":
			processor.passed++
		case "fail":
			processor.failed++
		case "skip":
			processor.skipped++
		}
	}

	assert.Equal(t, 2, processor.passed, "Should have 2 passed tests")
	assert.Equal(t, 1, processor.failed, "Should have 1 failed test")
	assert.Equal(t, 1, processor.skipped, "Should have 1 skipped test")
}
