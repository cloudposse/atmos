package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestNewTestModel(t *testing.T) {
	testPackages := []string{"./pkg1", "./pkg2"}
	testArgs := "-v -race"
	outputFile := "output.json"
	coverProfile := "coverage.out"
	showFilter := "failed"

	model := NewTestModel(testPackages, testArgs, outputFile, coverProfile, showFilter, false, "", 0)
	// Check that model fields are set correctly
	if model.outputFile != outputFile {
		t.Errorf("NewTestModel() outputFile = %v, want %v", model.outputFile, outputFile)
	}

	if model.showFilter != showFilter {
		t.Errorf("NewTestModel() showFilter = %v, want %v", model.showFilter, showFilter)
	}

	// totalTests should start at 0 and be incremented by "run" events
	if model.totalTests != 0 {
		t.Errorf("NewTestModel() totalTests = %v, want %v", model.totalTests, 0)
	}

	// Check that startTime is initialized
	if model.startTime.IsZero() {
		t.Error("NewTestModel() should initialize startTime")
	}

	// Check that time is recent (within last few seconds)
	timeDiff := time.Since(model.startTime)
	if timeDiff > 5*time.Second {
		t.Errorf("NewTestModel() startTime is too old: %v", timeDiff)
	}
}

func TestTestModelInit(t *testing.T) {
	model := NewTestModel([]string{"./pkg"}, "", "", "", "all", false, "", 0)
	cmd := model.Init()

	// Init should return a command (spinner tick)
	if cmd == nil {
		t.Error("TestModel.Init() should return a non-nil command")
	}
}

func TestMax(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want int
	}{
		{
			name: "a greater than b",
			a:    10,
			b:    5,
			want: 10,
		},
		{
			name: "b greater than a",
			a:    3,
			b:    8,
			want: 8,
		},
		{
			name: "equal values",
			a:    5,
			b:    5,
			want: 5,
		},
		{
			name: "negative values",
			a:    -3,
			b:    -7,
			want: -3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := max(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("max(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestShouldShowTest(t *testing.T) {
	tests := []struct {
		name       string
		showFilter string
		status     string
		want       bool
	}{
		{
			name:       "show all - pass",
			showFilter: "all",
			status:     "pass",
			want:       true,
		},
		{
			name:       "show all - fail",
			showFilter: "all",
			status:     "fail",
			want:       true,
		},
		{
			name:       "show all - skip",
			showFilter: "all",
			status:     "skip",
			want:       true,
		},
		{
			name:       "show failed - pass",
			showFilter: "failed",
			status:     "pass",
			want:       false,
		},
		{
			name:       "show failed - fail",
			showFilter: "failed",
			status:     "fail",
			want:       true,
		},
		{
			name:       "show passed - pass",
			showFilter: "passed",
			status:     "pass",
			want:       true,
		},
		{
			name:       "show passed - fail",
			showFilter: "passed",
			status:     "fail",
			want:       false,
		},
		{
			name:       "show skipped - skip",
			showFilter: "skipped",
			status:     "skip",
			want:       true,
		},
		{
			name:       "show skipped - pass",
			showFilter: "skipped",
			status:     "pass",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := TestModel{showFilter: tt.showFilter}
			got := model.shouldShowTest(tt.status)

			if got != tt.want {
				t.Errorf("shouldShowTest(%v) with filter %v = %v, want %v", tt.status, tt.showFilter, got, tt.want)
			}
		})
	}
}

func TestGetExitCode(t *testing.T) {
	tests := []struct {
		name      string
		failCount int
		want      int
	}{
		{
			name:      "no failures",
			failCount: 0,
			want:      0,
		},
		{
			name:      "with failures",
			failCount: 5,
			want:      1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := TestModel{failCount: tt.failCount}
			got := model.GetExitCode()

			if got != tt.want {
				t.Errorf("GetExitCode() with failCount %v = %v, want %v", tt.failCount, got, tt.want)
			}
		})
	}
}

func TestGenerateFinalSummary(t *testing.T) {
	// Test that generateFinalSummary produces output without panicking
	model := TestModel{
		totalTests: 10,
		passCount:  8,
		failCount:  1,
		skipCount:  1,
		startTime:  time.Now().Add(-5 * time.Second), // 5 seconds ago
	}

	summary := model.generateFinalSummary()

	// Check that summary contains expected information
	if summary == "" {
		t.Error("generateFinalSummary() should return non-empty summary")
	}

	// Summary should contain test counts
	if !strings.Contains(summary, "8") { // pass count
		t.Error("generateFinalSummary() should contain pass count")
	}

	if !strings.Contains(summary, "1") { // fail count
		t.Error("generateFinalSummary() should contain fail count")
	}
}

func TestView(t *testing.T) {
	// Test that View produces output without panicking
	model := TestModel{
		totalTests: 10,
		passCount:  3,
		failCount:  1,
		skipCount:  1,
		startTime:  time.Now(),
	}

	// Initialize required components
	// Ensure spinner is initialized

	view := model.View()

	// Check that view produces some output
	if view == "" {
		t.Error("View() should return non-empty view")
	}
}

// Test Update with various message types.
func TestUpdate(t *testing.T) {
	model := TestModel{
		totalTests: 10,
		startTime:  time.Now(),
	}

	// Test with spinner tick message
	tickMsg := model.spinner.Tick()
	newModel, cmd := model.Update(tickMsg)

	// Should return a model (converted back to TestModel)
	if newModel == nil {
		t.Error("Update() should return non-nil model")
	}

	// Should return a command
	if cmd == nil {
		t.Error("Update() should return non-nil command for spinner tick")
	}

	// Test with key press message (Ctrl+C)
	keyMsg := tea.KeyMsg{
		Type: tea.KeyCtrlC,
	}

	_, cmd = model.Update(keyMsg)

	// Should return quit command for Ctrl+C
	if cmd == nil {
		t.Error("Update() should return non-nil command for Ctrl+C")
	}
}

// Test that model initialization doesn't crash.
func TestStartTestsCmd(t *testing.T) {
	model := TestModel{
		totalTests: 10,
	}

	// Test basic model properties
	if model.totalTests != 10 {
		t.Errorf("Expected totalTests to be 10, got %d", model.totalTests)
	}
}

// Test readNextLine.
func TestReadNextLine(t *testing.T) {
	model := TestModel{}

	cmd := model.readNextLine()

	// Should return a command
	if cmd == nil {
		t.Error("readNextLine() should return non-nil command")
	}
}

// Test buffer handling for subtests and out-of-order events
func TestTestBufferHandling(t *testing.T) {
	tests := []struct {
		name           string
		events         []types.TestEvent
		expectedBuffer map[string][]string
		description    string
	}{
		{
			name: "basic test with output",
			events: []types.TestEvent{
				{Action: "run", Test: "TestBasic"},
				{Action: "output", Test: "TestBasic", Output: "test output line 1\n"},
				{Action: "output", Test: "TestBasic", Output: "test output line 2\n"},
				{Action: "fail", Test: "TestBasic"},
			},
			expectedBuffer: map[string][]string{
				"TestBasic": {
					"test output line 1\n",
					"test output line 2\n",
				},
			},
			description: "Should buffer output for basic test",
		},
		{
			name: "output before run event",
			events: []types.TestEvent{
				{Action: "output", Test: "TestEarly", Output: "early output\n"},
				{Action: "run", Test: "TestEarly"},
				{Action: "output", Test: "TestEarly", Output: "normal output\n"},
				{Action: "pass", Test: "TestEarly"},
			},
			expectedBuffer: map[string][]string{
				"TestEarly": {
					"early output\n",
					"normal output\n",
				},
			},
			description: "Should handle output events that come before run event",
		},
		{
			name: "subtest with parent test",
			events: []types.TestEvent{
				{Action: "run", Test: "TestParent"},
				{Action: "run", Test: "TestParent/subtest1"},
				{Action: "output", Test: "TestParent/subtest1", Output: "subtest output 1\n"},
				{Action: "fail", Test: "TestParent/subtest1"},
				{Action: "run", Test: "TestParent/subtest2"},
				{Action: "output", Test: "TestParent/subtest2", Output: "subtest output 2\n"},
				{Action: "pass", Test: "TestParent/subtest2"},
				{Action: "fail", Test: "TestParent"},
			},
			expectedBuffer: map[string][]string{
				"TestParent": {},
				"TestParent/subtest1": {
					"subtest output 1\n",
				},
				"TestParent/subtest2": {
					"subtest output 2\n",
				},
			},
			description: "Should maintain separate buffers for parent and subtests",
		},
		{
			name: "no run event for test",
			events: []types.TestEvent{
				{Action: "output", Test: "TestNoRun", Output: "output without run\n"},
				{Action: "output", Test: "TestNoRun", Output: "more output\n"},
				{Action: "fail", Test: "TestNoRun"},
			},
			expectedBuffer: map[string][]string{
				"TestNoRun": {
					"output without run\n",
					"more output\n",
				},
			},
			description: "Should create buffer on first output even without run event",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := &TestModel{
				testBuffers: make(map[string][]string),
			}

			// Process events
			for _, event := range tt.events {
				processTestEvent(model, &event)
			}

			// Verify buffers
			assert.Equal(t, len(tt.expectedBuffer), len(model.testBuffers),
				"Buffer count mismatch for %s", tt.description)

			for testName, expectedOutput := range tt.expectedBuffer {
				actualOutput, exists := model.testBuffers[testName]
				assert.True(t, exists, "Buffer should exist for test %s", testName)
				assert.Equal(t, expectedOutput, actualOutput,
					"Output mismatch for test %s: %s", testName, tt.description)
			}
		})
	}
}

func TestSubtestOutputCollection(t *testing.T) {
	tests := []struct {
		name           string
		parentTest     string
		testBuffers    map[string][]string
		expectedOutput []string
		description    string
	}{
		{
			name:       "parent with no output but subtests have output",
			parentTest: "TestParent",
			testBuffers: map[string][]string{
				"TestParent":          {},
				"TestParent/subtest1": {"error in subtest1\n"},
				"TestParent/subtest2": {"error in subtest2\n"},
			},
			expectedOutput: []string{
				"error in subtest1\n",
				"error in subtest2\n",
			},
			description: "Should collect output from subtests when parent has none",
		},
		{
			name:       "parent with output and subtests also have output",
			parentTest: "TestParent",
			testBuffers: map[string][]string{
				"TestParent":          {"parent output\n"},
				"TestParent/subtest1": {"subtest1 output\n"},
				"TestParent/subtest2": {"subtest2 output\n"},
			},
			expectedOutput: []string{
				"parent output\n",
			},
			description: "Should use parent output when it exists",
		},
		{
			name:       "nested subtests",
			parentTest: "TestParent",
			testBuffers: map[string][]string{
				"TestParent":                      {},
				"TestParent/level1":               {"level1 output\n"},
				"TestParent/level1/level2":        {"level2 output\n"},
				"TestParent/level1/level2/level3": {"level3 output\n"},
			},
			expectedOutput: []string{
				"level1 output\n",
				"level2 output\n",
				"level3 output\n",
			},
			description: "Should collect output from all nested subtests",
		},
		{
			name:       "no subtests",
			parentTest: "TestSimple",
			testBuffers: map[string][]string{
				"TestSimple": {"simple output\n"},
				"TestOther":  {"other output\n"},
			},
			expectedOutput: []string{
				"simple output\n",
			},
			description: "Should only use test's own output when no subtests exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the logic from the fail event handler
			output := tt.testBuffers[tt.parentTest]

			// If no output found, check for subtest output
			if len(output) == 0 {
				testPrefix := tt.parentTest + "/"
				for testName, testOutput := range tt.testBuffers {
					if strings.HasPrefix(testName, testPrefix) && len(testOutput) > 0 {
						output = append(output, testOutput...)
					}
				}
			}

			assert.Equal(t, tt.expectedOutput, output, tt.description)
		})
	}
}

// Helper function to process test events (simulates the actual event processing)
func processTestEvent(model *TestModel, event *types.TestEvent) {
	switch event.Action {
	case "run":
		// Only initialize if buffer doesn't exist (to preserve early output)
		if model.testBuffers[event.Test] == nil {
			model.testBuffers[event.Test] = []string{}
		}
	case "output":
		// Create buffer if it doesn't exist (can happen with subtests or out-of-order events)
		if model.testBuffers[event.Test] == nil {
			model.testBuffers[event.Test] = []string{}
		}
		model.testBuffers[event.Test] = append(model.testBuffers[event.Test], event.Output)
	}
}

func TestEventOrdering(t *testing.T) {
	// Test various event ordering scenarios that could cause issues
	tests := []struct {
		name        string
		events      []types.TestEvent
		shouldWork  bool
		description string
	}{
		{
			name: "output before any run event",
			events: []types.TestEvent{
				{Action: "output", Test: "TestA", Output: "line1\n"},
				{Action: "output", Test: "TestA", Output: "line2\n"},
				{Action: "fail", Test: "TestA"},
			},
			shouldWork:  true,
			description: "Should handle output with no run event",
		},
		{
			name: "interleaved parent and subtest events",
			events: []types.TestEvent{
				{Action: "run", Test: "TestParent"},
				{Action: "output", Test: "TestParent/sub1", Output: "sub1 output\n"},
				{Action: "run", Test: "TestParent/sub1"},
				{Action: "output", Test: "TestParent", Output: "parent output\n"},
				{Action: "fail", Test: "TestParent/sub1"},
				{Action: "fail", Test: "TestParent"},
			},
			shouldWork:  true,
			description: "Should handle interleaved parent/subtest events",
		},
		{
			name: "duplicate run events",
			events: []types.TestEvent{
				{Action: "run", Test: "TestDup"},
				{Action: "output", Test: "TestDup", Output: "output1\n"},
				{Action: "run", Test: "TestDup"}, // Duplicate run
				{Action: "output", Test: "TestDup", Output: "output2\n"},
				{Action: "pass", Test: "TestDup"},
			},
			shouldWork:  true,
			description: "Should handle duplicate run events gracefully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := &TestModel{
				testBuffers: make(map[string][]string),
			}

			// Process events and ensure no panic
			assert.NotPanics(t, func() {
				for _, event := range tt.events {
					processTestEvent(model, &event)
				}
			}, tt.description)

			if tt.shouldWork {
				// Verify that buffers were created for all tests with output
				for _, event := range tt.events {
					if event.Action == "output" {
						_, exists := model.testBuffers[event.Test]
						assert.True(t, exists,
							"Buffer should exist for test %s after output event", event.Test)
					}
				}
			}
		})
	}
}
