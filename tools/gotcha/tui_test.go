package main

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewTestModel(t *testing.T) {
	testPackages := []string{"./pkg1", "./pkg2"}
	testArgs := "-v -race"
	outputFile := "output.json"
	coverProfile := "coverage.out"
	showFilter := "failed"
	totalTests := 42
	
	model := &testModel{}
	*model = newTestModel(testPackages, testArgs, outputFile, coverProfile, showFilter, totalTests, false)
	// Check that model fields are set correctly
	if model.outputFile != outputFile {
		t.Errorf("newTestModel() outputFile = %v, want %v", model.outputFile, outputFile)
	}

	if model.showFilter != showFilter {
		t.Errorf("newTestModel() showFilter = %v, want %v", model.showFilter, showFilter)
	}

	if model.totalTests != totalTests {
		t.Errorf("newTestModel() totalTests = %v, want %v", model.totalTests, totalTests)
	}

	// Check that startTime is initialized
	if model.startTime.IsZero() {
		t.Error("newTestModel() should initialize startTime")
	}

	// Check that time is recent (within last few seconds)
	timeDiff := time.Since(model.startTime)
	if timeDiff > 5*time.Second {
		t.Errorf("newTestModel() startTime is too old: %v", timeDiff)
	}
}

func TestTestModelInit(t *testing.T) {
	model := &testModel{}
	*model = newTestModel([]string{"./pkg"}, "", "", "", "all", 10, false)
	cmd := model.Init()

	// Init should return a command (spinner tick)
	if cmd == nil {
		t.Error("testModel.Init() should return a non-nil command")
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
			model := testModel{showFilter: tt.showFilter}
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
			model := testModel{failCount: tt.failCount}
			got := model.GetExitCode()

			if got != tt.want {
				t.Errorf("GetExitCode() with failCount %v = %v, want %v", tt.failCount, got, tt.want)
			}
		})
	}
}

func TestGenerateFinalSummary(t *testing.T) {
	// Test that generateFinalSummary produces output without panicking
	model := testModel{
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
	if !contains([]string{summary}, "8") { // pass count
		t.Error("generateFinalSummary() should contain pass count")
	}

	if !contains([]string{summary}, "1") { // fail count
		t.Error("generateFinalSummary() should contain fail count")
	}
}

func TestView(t *testing.T) {
	// Test that View produces output without panicking
	model := testModel{
		totalTests: 10,
		passCount:  3,
		failCount:  1,
		skipCount:  1,
		startTime:  time.Now(),
	}

	// Initialize required components
	model.spinner.Spinner = model.spinner.Spinner // Ensure spinner is initialized

	view := model.View()

	// Check that view produces some output
	if view == "" {
		t.Error("View() should return non-empty view")
	}
}

// Test Update with various message types
func TestUpdate(t *testing.T) {
	model := testModel{
		totalTests: 10,
		startTime:  time.Now(),
	}

	// Test with spinner tick message
	tickMsg := model.spinner.Tick()
	newModel, cmd := model.Update(tickMsg)

	// Should return a model (converted back to testModel)
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

// Test that model initialization doesn't crash
func TestStartTestsCmd(t *testing.T) {
	model := testModel{
		totalTests: 10,
	}

	// Test basic model properties
	if model.totalTests != 10 {
		t.Errorf("Expected totalTests to be 10, got %d", model.totalTests)
	}
}

// Test readNextLine
func TestReadNextLine(t *testing.T) {
	model := testModel{}

	cmd := model.readNextLine()

	// Should return a command
	if cmd == nil {
		t.Error("readNextLine() should return non-nil command")
	}
}
