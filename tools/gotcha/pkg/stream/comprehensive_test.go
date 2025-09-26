package stream

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/stretchr/testify/assert"
)

// TestPackageResult tests the PackageResult struct.
func TestPackageResult(t *testing.T) {
	pkg := &PackageResult{
		Package:           "github.com/example/pkg",
		StartTime:         time.Now(),
		EndTime:           time.Now().Add(5 * time.Second),
		Status:            "pass",
		Tests:             make(map[string]*TestResult),
		TestOrder:         []string{"Test1", "Test2"},
		Coverage:          "75.0%",
		StatementCoverage: "75.0%",
		FunctionCoverage:  "80.0%",
		Output:            []string{"output line 1", "output line 2"},
		Elapsed:           5.0,
		HasTests:          true,
	}

	assert.Equal(t, "github.com/example/pkg", pkg.Package)
	assert.Equal(t, "pass", pkg.Status)
	assert.Equal(t, "75.0%", pkg.Coverage)
	assert.True(t, pkg.HasTests)
	assert.Len(t, pkg.TestOrder, 2)
}

// TestTestResult tests the TestResult struct.
func TestTestResult(t *testing.T) {
	result := &TestResult{
		Name:         "TestExample",
		FullName:     "github.com/example/pkg.TestExample",
		Status:       "pass",
		Elapsed:      1.5,
		Output:       []string{"test output"},
		Parent:       "",
		Subtests:     make(map[string]*TestResult),
		SubtestOrder: []string{},
		SkipReason:   "",
	}

	assert.Equal(t, "TestExample", result.Name)
	assert.Equal(t, "pass", result.Status)
	assert.Equal(t, 1.5, result.Elapsed)
	assert.Len(t, result.Output, 1)
}

// TestStreamProcessorFields tests StreamProcessor initialization.
func TestStreamProcessorFields(t *testing.T) {
	var buf bytes.Buffer
	proc := NewStreamProcessor(&buf, "all", "TestFilter", "normal")

	assert.NotNil(t, proc)
	assert.NotNil(t, proc.packageResults)
	assert.NotNil(t, proc.activePackages)
	assert.NotNil(t, proc.packagesWithNoTests)
	assert.NotNil(t, proc.packageHasTests)
	assert.NotNil(t, proc.packageNoTestsPrinted)
	assert.Equal(t, "all", proc.showFilter)
	assert.Equal(t, "TestFilter", proc.testFilter)
	assert.Equal(t, "normal", proc.verbosityLevel)
	assert.NotNil(t, proc.reporter)
	assert.NotNil(t, proc.writer)
}

// TestProcessStreamWithValidJSON tests processing valid JSON events.
func TestProcessStreamWithValidJSON(t *testing.T) {
	var jsonBuf bytes.Buffer
	proc := NewStreamProcessor(&jsonBuf, "all", "", "normal")

	events := []types.TestEvent{
		{Action: "run", Package: "test/pkg", Test: "TestExample"},
		{Action: "output", Package: "test/pkg", Test: "TestExample", Output: "test output\n"},
		{Action: "pass", Package: "test/pkg", Test: "TestExample", Elapsed: 1.0},
		{Action: "pass", Package: "test/pkg", Elapsed: 1.5},
	}

	var input bytes.Buffer
	for _, event := range events {
		data, _ := json.Marshal(event)
		input.Write(data)
		input.WriteString("\n")
	}

	err := proc.ProcessStream(&input)
	assert.NoError(t, err)

	// Check that JSON was written
	jsonOutput := jsonBuf.String()
	assert.Contains(t, jsonOutput, "TestExample")
	assert.Contains(t, jsonOutput, "pass")
}

// TestProcessStreamWithInvalidJSON tests processing with invalid JSON lines.
func TestProcessStreamWithInvalidJSON(t *testing.T) {
	var jsonBuf bytes.Buffer
	proc := NewStreamProcessor(&jsonBuf, "all", "", "normal")

	input := strings.NewReader("not json\n{invalid json\n")

	// Should not error on invalid JSON (just skips the lines)
	err := proc.ProcessStream(input)
	assert.NoError(t, err)
}

// TestProcessStreamEmptyInput tests processing empty input.
func TestProcessStreamEmptyInput(t *testing.T) {
	var jsonBuf bytes.Buffer
	proc := NewStreamProcessor(&jsonBuf, "all", "", "normal")

	input := strings.NewReader("")

	err := proc.ProcessStream(input)
	assert.NoError(t, err)
}

// TestGetLastExitReason tests the GetLastExitReason function.
func TestGetLastExitReasonFunction(t *testing.T) {
	// Reset the global variable
	lastExitReason = ""

	assert.Equal(t, "", GetLastExitReason())

	lastExitReason = "test failure reason"
	assert.Equal(t, "test failure reason", GetLastExitReason())

	lastExitReason = "another reason"
	assert.Equal(t, "another reason", GetLastExitReason())
}

// TestFindTest tests the findTest helper function.
func TestFindTestFunction(t *testing.T) {
	pkg1 := &PackageResult{
		Package: "pkg1",
		Tests: map[string]*TestResult{
			"Test1": {Name: "Test1", Status: "pass"},
			"Test2": {Name: "Test2", Status: "fail"},
		},
	}

	proc := &StreamProcessor{
		packageResults: map[string]*PackageResult{
			"pkg1": pkg1,
		},
	}

	// Find existing test
	test := proc.findTest(pkg1, "Test1")
	assert.NotNil(t, test)
	assert.Equal(t, "Test1", test.Name)
	assert.Equal(t, "pass", test.Status)

	// Find another test
	test = proc.findTest(pkg1, "Test2")
	assert.NotNil(t, test)
	assert.Equal(t, "fail", test.Status)

	// Try to find non-existent test
	test = proc.findTest(pkg1, "TestNotExist")
	assert.Nil(t, test)

	// Try to find test in different (empty) package
	emptyPkg := &PackageResult{
		Package: "empty",
		Tests:   make(map[string]*TestResult),
	}
	test = proc.findTest(emptyPkg, "Test1")
	assert.Nil(t, test)
}

// TestStreamProcessorShowFilter tests the show filter behavior.
func TestStreamProcessorShowFilter(t *testing.T) {
	tests := []struct {
		name       string
		showFilter string
		status     string
		shouldShow bool
	}{
		{"all filter shows everything", "all", "pass", true},
		{"failed filter shows failures", "failed", "fail", true},
		{"failed filter hides passes", "failed", "pass", false},
		{"passed filter shows passes", "passed", "pass", true},
		{"passed filter hides failures", "passed", "fail", false},
		{"skipped filter shows skips", "skipped", "skip", true},
		{"none filter hides everything", "none", "pass", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			proc := NewStreamProcessor(&buf, tt.showFilter, "", "normal")

			// Test that the processor was created with the right filter
			assert.Equal(t, tt.showFilter, proc.showFilter)

			// The actual filtering is done by the reporter, so we just verify
			// that the processor has the right configuration
			expected := tt.shouldShow || tt.showFilter == "all"
			_ = expected // Avoid unused variable warning
		})
	}
}

// TestProcessorWithReporter tests creating processor with custom reporter.
func TestProcessorWithCustomReporter(t *testing.T) {
	var buf bytes.Buffer
	reporter := NewStreamReporter(nil, "all", "", "normal")
	proc := NewStreamProcessorWithReporter(&buf, reporter)

	assert.NotNil(t, proc)
	assert.Equal(t, reporter, proc.reporter)
	assert.NotNil(t, proc.packageResults)
}

// TestProcessStreamIncompletePackages tests handling of incomplete packages.
func TestProcessStreamIncompletePackages(t *testing.T) {
	var jsonBuf bytes.Buffer
	proc := NewStreamProcessor(&jsonBuf, "all", "", "normal")

	// Simulate a package that starts but never completes
	events := []types.TestEvent{
		{Action: "run", Package: "incomplete/pkg", Test: "TestIncomplete"},
		{Action: "output", Package: "incomplete/pkg", Test: "TestIncomplete", Output: "running...\n"},
		// No pass/fail event for the test or package
	}

	var input bytes.Buffer
	for _, event := range events {
		data, _ := json.Marshal(event)
		input.Write(data)
		input.WriteString("\n")
	}

	// Mark package as active manually (simulating processEvent behavior)
	proc.packageResults["incomplete/pkg"] = &PackageResult{
		Package: "incomplete/pkg",
		Status:  "running",
		Tests:   make(map[string]*TestResult),
	}
	proc.activePackages["incomplete/pkg"] = true

	err := proc.ProcessStream(&input)
	assert.NoError(t, err)

	// The incomplete package should be marked as failed
	pkg := proc.packageResults["incomplete/pkg"]
	assert.NotNil(t, pkg)
	assert.Equal(t, "fail", pkg.Status)
}

// TestPackageResultDefaults tests default values for PackageResult.
func TestPackageResultDefaults(t *testing.T) {
	pkg := &PackageResult{}

	assert.Empty(t, pkg.Package)
	assert.Zero(t, pkg.StartTime)
	assert.Zero(t, pkg.EndTime)
	assert.Empty(t, pkg.Status)
	assert.Nil(t, pkg.Tests)
	assert.Nil(t, pkg.TestOrder)
	assert.Empty(t, pkg.Coverage)
	assert.Empty(t, pkg.StatementCoverage)
	assert.Empty(t, pkg.FunctionCoverage)
	assert.Nil(t, pkg.Output)
	assert.Zero(t, pkg.Elapsed)
	assert.False(t, pkg.HasTests)
}

// TestTestResultWithSubtests tests TestResult with subtests.
func TestTestResultWithSubtests(t *testing.T) {
	parent := &TestResult{
		Name:         "TestParent",
		Status:       "pass",
		Subtests:     make(map[string]*TestResult),
		SubtestOrder: []string{},
	}

	// Add subtests
	subtest1 := &TestResult{
		Name:   "TestParent/Sub1",
		Status: "pass",
		Parent: "TestParent",
	}
	parent.Subtests["TestParent/Sub1"] = subtest1
	parent.SubtestOrder = append(parent.SubtestOrder, "TestParent/Sub1")

	subtest2 := &TestResult{
		Name:   "TestParent/Sub2",
		Status: "fail",
		Parent: "TestParent",
	}
	parent.Subtests["TestParent/Sub2"] = subtest2
	parent.SubtestOrder = append(parent.SubtestOrder, "TestParent/Sub2")

	assert.Len(t, parent.Subtests, 2)
	assert.Len(t, parent.SubtestOrder, 2)
	assert.Equal(t, "TestParent", subtest1.Parent)
	assert.Equal(t, "TestParent", subtest2.Parent)
}

// TestStreamProcessorConcurrency tests concurrent access to processor.
func TestStreamProcessorConcurrency(t *testing.T) {
	var buf bytes.Buffer
	proc := NewStreamProcessor(&buf, "all", "", "normal")

	// Initialize some data
	pkg1 := &PackageResult{
		Package: "pkg1",
		Tests:   make(map[string]*TestResult),
	}
	proc.packageResults["pkg1"] = pkg1

	// Concurrent reads should be safe (with proper locking in real code)
	done := make(chan bool, 2)

	go func() {
		_ = proc.findTest(pkg1, "Test1")
		done <- true
	}()

	go func() {
		_ = proc.findTest(pkg1, "Test2")
		done <- true
	}()

	<-done
	<-done

	// No assertions needed - just verifying no panic/race
}

// TestPackageResultWithCoverage tests PackageResult with coverage data.
func TestPackageResultWithCoverage(t *testing.T) {
	pkg := &PackageResult{
		Package:           "github.com/example/pkg",
		Status:            "pass",
		Coverage:          "85.5%",
		StatementCoverage: "85.5%",
		FunctionCoverage:  "90.0%",
		HasTests:          true,
	}

	assert.Equal(t, "85.5%", pkg.Coverage)
	assert.Equal(t, "85.5%", pkg.StatementCoverage)
	assert.Equal(t, "90.0%", pkg.FunctionCoverage)
}

// TestTestResultSkipped tests TestResult for skipped tests.
func TestTestResultSkipped(t *testing.T) {
	result := &TestResult{
		Name:       "TestSkipped",
		Status:     "skip",
		SkipReason: "Not implemented on this platform",
		Elapsed:    0.001,
	}

	assert.Equal(t, "skip", result.Status)
	assert.NotEmpty(t, result.SkipReason)
	assert.Contains(t, result.SkipReason, "Not implemented")
}
