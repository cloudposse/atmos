package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
		// "all" filter shows everything
		{"all filter with pass", "all", "pass", true},
		{"all filter with fail", "all", "fail", true},
		{"all filter with skip", "all", "skip", true},

		// "failed" filter only shows failures
		{"failed filter with pass", "failed", "pass", false},
		{"failed filter with fail", "failed", "fail", true},
		{"failed filter with skip", "failed", "skip", false},

		// "passed" filter only shows passes
		{"passed filter with pass", "passed", "pass", true},
		{"passed filter with fail", "passed", "fail", false},
		{"passed filter with skip", "passed", "skip", false},

		// "skipped" filter only shows skipped
		{"skipped filter with pass", "skipped", "pass", false},
		{"skipped filter with fail", "skipped", "fail", false},
		{"skipped filter with skip", "skipped", "skip", true},

		// "collapsed" filter only shows failures
		{"collapsed filter with pass", "collapsed", "pass", false},
		{"collapsed filter with fail", "collapsed", "fail", true},
		{"collapsed filter with skip", "collapsed", "skip", false},

		// "none" filter shows nothing
		{"none filter with pass", "none", "pass", false},
		{"none filter with fail", "none", "fail", false},
		{"none filter with skip", "none", "skip", false},

		// Unknown filter defaults to showing everything
		{"unknown filter with pass", "unknown", "pass", true},
		{"unknown filter with fail", "unknown", "fail", true},
		{"unknown filter with skip", "unknown", "skip", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := &TestModel{
				showFilter: tt.showFilter,
			}
			got := model.shouldShowTest(tt.status)
			if got != tt.want {
				t.Errorf("shouldShowTest() with filter=%v status=%v = %v, want %v",
					tt.showFilter, tt.status, got, tt.want)
			}
		})
	}
}

func TestGetExitCode(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		aborted  bool
		want     int
	}{
		{"successful completion", 0, false, 0},
		{"test failure", 1, false, 1},
		{"multiple failures", 2, false, 2},
		{"aborted test", 0, true, 130},
		{"aborted with failures", 1, true, 130},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := &TestModel{
				exitCode: tt.exitCode,
				aborted:  tt.aborted,
			}
			got := model.GetExitCode()
			if got != tt.want {
				t.Errorf("GetExitCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateFinalSummary(t *testing.T) {
	model := &TestModel{
		passed:    10,
		failed:    2,
		skipped:   3,
		startTime: time.Now().Add(-5 * time.Second),
		packageResults: map[string]*PackageResult{
			"pkg1": {Coverage: "80.5%"},
			"pkg2": {Coverage: "90.0%"},
			"pkg3": {Coverage: "0.0%"}, // Should be excluded from average
		},
	}

	summary := model.GenerateFinalSummary()

	// Check for expected content in summary
	assert.Contains(t, summary, "Passed:  10")
	assert.Contains(t, summary, "Failed:  2")
	assert.Contains(t, summary, "Skipped: 3")
	assert.Contains(t, summary, "Total:     15")
	assert.Contains(t, summary, "Coverage:  85.2%") // Average of 80.5 and 90.0
	assert.Contains(t, summary, "Tests completed in")
}

func TestView(t *testing.T) {
	model := &TestModel{
		width:    80,
		height:   24,
		passed:   5,
		failed:   1,
		skipped:  0,
		done:     false,
		exitCode: 0,
	}

	view := model.View()

	// Check for essential UI elements
	assert.Contains(t, view, "🧪 Go Test Runner")
	assert.Contains(t, view, "Running tests...")
	assert.Contains(t, view, "5 passed")
	assert.Contains(t, view, "1 failed")
}

func TestUpdate(t *testing.T) {
	model := &TestModel{
		width:  80,
		height: 24,
	}

	// Test window resize
	newModel, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updatedModel := newModel.(*TestModel)
	assert.Equal(t, 100, updatedModel.width)
	assert.Equal(t, 30, updatedModel.height)

	// Test quit key
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	assert.NotNil(t, cmd)

	// Test test complete
	model.done = false
	newModel, _ = model.Update(testCompleteMsg{exitCode: 1})
	updatedModel = newModel.(*TestModel)
	assert.True(t, updatedModel.done)
	assert.Equal(t, 1, updatedModel.exitCode)
}

func TestStartTestsCmd(t *testing.T) {
	model := &TestModel{
		testPackages: []string{"./pkg"},
		testArgs:     "-v",
		coverProfile: "coverage.out",
	}

	// This test would need mocking of exec.Command
	// For now, just ensure the function exists
	cmd := model.startTestsCmd()
	assert.NotNil(t, cmd)
}

func TestReadNextLine(t *testing.T) {
	model := &TestModel{}

	// Without a scanner, should return nil
	cmd := model.readNextLine()
	msg := cmd()
	assert.Nil(t, msg)
}
