package tui

import (
	"strings"
	"testing"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestTestBufferHandling(t *testing.T) {
	model := &TestModel{
		buffers:        make(map[string][]string),
		packageResults: make(map[string]*PackageResult),
		activePackages: make(map[string]bool),
		subtestStats:   make(map[string]*SubtestStats),
		packagesWithNoTests: make(map[string]bool),
		packageHasTests:     make(map[string]bool),
	}

	// Process a test run event
	event := &types.TestEvent{
		Action:  "run",
		Package: "test/pkg",
		Test:    "TestExample",
	}
	model.processEvent(event)

	// Verify test was created
	pkg := model.packageResults["test/pkg"]
	assert.NotNil(t, pkg)
	assert.NotNil(t, pkg.Tests["TestExample"])
	assert.Equal(t, "running", pkg.Tests["TestExample"].Status)

	// Process output event
	outputEvent := &types.TestEvent{
		Action:  "output",
		Package: "test/pkg",
		Test:    "TestExample",
		Output:  "=== RUN   TestExample\n",
	}
	model.processEvent(outputEvent)

	// Check output was captured
	assert.Len(t, pkg.Tests["TestExample"].Output, 1)
	assert.Equal(t, "=== RUN   TestExample\n", pkg.Tests["TestExample"].Output[0])

	// Process pass event
	passEvent := &types.TestEvent{
		Action:  "pass",
		Package: "test/pkg",
		Test:    "TestExample",
		Elapsed: 0.5,
	}
	model.processEvent(passEvent)

	// Verify test passed
	assert.Equal(t, "pass", pkg.Tests["TestExample"].Status)
	assert.Equal(t, 0.5, pkg.Tests["TestExample"].Elapsed)
	assert.Equal(t, 1, model.passCount)
}

func TestSubtestOutputCollection(t *testing.T) {
	model := &TestModel{
		buffers:        make(map[string][]string),
		packageResults: make(map[string]*PackageResult),
		activePackages: make(map[string]bool),
		subtestStats:   make(map[string]*SubtestStats),
		packagesWithNoTests: make(map[string]bool),
		packageHasTests:     make(map[string]bool),
	}

	// Create parent test
	model.processEvent(&types.TestEvent{
		Action:  "run",
		Package: "test/pkg",
		Test:    "TestWithSubtests",
	})

	// Create subtest
	model.processEvent(&types.TestEvent{
		Action:  "run",
		Package: "test/pkg",
		Test:    "TestWithSubtests/Subtest1",
	})

	// Add output to subtest
	model.processEvent(&types.TestEvent{
		Action:  "output",
		Package: "test/pkg",
		Test:    "TestWithSubtests/Subtest1",
		Output:  "subtest output line 1\n",
	})

	// Verify subtest was created and has output
	pkg := model.packageResults["test/pkg"]
	parent := pkg.Tests["TestWithSubtests"]
	assert.NotNil(t, parent)
	
	subtest := parent.Subtests["TestWithSubtests/Subtest1"]
	assert.NotNil(t, subtest)
	assert.Len(t, subtest.Output, 1)
	assert.Equal(t, "subtest output line 1\n", subtest.Output[0])

	// Pass the subtest
	model.processEvent(&types.TestEvent{
		Action:  "pass",
		Package: "test/pkg",
		Test:    "TestWithSubtests/Subtest1",
		Elapsed: 0.1,
	})

	// Verify subtest passed and stats were updated
	assert.Equal(t, "pass", subtest.Status)
	assert.NotNil(t, model.subtestStats["TestWithSubtests"])
	assert.Contains(t, model.subtestStats["TestWithSubtests"].passed, "TestWithSubtests/Subtest1")
}

func TestEventOrdering(t *testing.T) {
	model := &TestModel{
		buffers:        make(map[string][]string),
		packageResults: make(map[string]*PackageResult),
		activePackages: make(map[string]bool),
		subtestStats:   make(map[string]*SubtestStats),
		packagesWithNoTests: make(map[string]bool),
		packageHasTests:     make(map[string]bool),
	}

	// Process events in order
	events := []types.TestEvent{
		{Action: "start", Package: "test/pkg"},
		{Action: "run", Package: "test/pkg", Test: "Test1"},
		{Action: "run", Package: "test/pkg", Test: "Test2"},
		{Action: "pass", Package: "test/pkg", Test: "Test1", Elapsed: 0.1},
		{Action: "fail", Package: "test/pkg", Test: "Test2", Elapsed: 0.2},
		{Action: "fail", Package: "test/pkg", Elapsed: 0.3},
	}

	for _, event := range events {
		e := event // Capture range variable
		model.processEvent(&e)
	}

	// Verify package results
	pkg := model.packageResults["test/pkg"]
	assert.NotNil(t, pkg)
	assert.Equal(t, "fail", pkg.Status)
	assert.Len(t, pkg.Tests, 2)
	assert.Equal(t, "pass", pkg.Tests["Test1"].Status)
	assert.Equal(t, "fail", pkg.Tests["Test2"].Status)
	assert.Equal(t, 1, model.passCount)
	assert.Equal(t, 1, model.failCount)
}

func TestDisplayPackageResult(t *testing.T) {
	model := &TestModel{
		showFilter: "all",
		packagesWithNoTests: make(map[string]bool),
		subtestStats: make(map[string]*SubtestStats),
	}

	// Test package with no tests
	pkg := &PackageResult{
		Package:  "empty/pkg",
		Status:   "skip",
		HasTests: false,
		Tests:    make(map[string]*TestResult),
	}
	
	result := model.displayPackageResult(pkg)
	assert.Contains(t, result, "empty/pkg")
	assert.Contains(t, result, "No tests")

	// Test package with passed tests
	pkg2 := &PackageResult{
		Package:  "test/pkg",
		Status:   "pass",
		HasTests: true,
		Tests: map[string]*TestResult{
			"TestPassed": {
				Name:    "TestPassed",
				Status:  "pass",
				Elapsed: 0.5,
			},
		},
		TestOrder: []string{"TestPassed"},
		Coverage:  "85.5%",
	}

	result2 := model.displayPackageResult(pkg2)
	assert.Contains(t, result2, "test/pkg")
	assert.Contains(t, result2, "TestPassed")
	assert.Contains(t, result2, "All 1 tests passed")
	assert.Contains(t, result2, "85.5% coverage")
}

func TestGenerateSubtestProgress(t *testing.T) {
	model := &TestModel{}

	tests := []struct {
		name   string
		passed int
		total  int
		want   string
	}{
		{"all passed", 5, 5, strings.Repeat("●", 5)},
		{"none passed", 0, 5, ""},
		{"half passed", 2, 4, strings.Repeat("●", 2)},
		{"large numbers", 50, 100, ""}, // Should scale down to maxDots
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := model.generateSubtestProgress(tt.passed, tt.total)
			
			// For large numbers, just check it's not too long
			if tt.total > 10 {
				assert.LessOrEqual(t, len(result)/len("●"), 10)
			} else if tt.passed == 0 {
				// Check for empty string when no tests passed
				assert.Equal(t, "", result)
			} else {
				// Count the number of dots (accounting for ANSI codes)
				greenDots := strings.Count(result, "●")
				assert.GreaterOrEqual(t, greenDots, tt.passed)
			}
		})
	}
}

func TestExtractSkipReason(t *testing.T) {
	model := &TestModel{}
	
	tests := []struct {
		name       string
		output     string
		wantReason string
	}{
		{
			name:       "skip with t.Skip",
			output:     "skip_test.go:9: SKIP: Test requires external service",
			wantReason: "Test requires external service",
		},
		{
			name:       "skip with Skipf",
			output:     "    helpers_test.go:123: skipping: Database not available",
			wantReason: "Database not available",
		},
		{
			name:       "skip with SKIP prefix",
			output:     "SKIP Test is flaky on CI",
			wantReason: "Test is flaky on CI",
		},
		{
			name:       "skip header line",
			output:     "--- SKIP: TestExample (0.00s)",
			wantReason: "",
		},
		{
			name:       "regular output",
			output:     "Running test setup",
			wantReason: "",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test := &TestResult{}
			model.extractSkipReason(tt.output, test)
			assert.Equal(t, tt.wantReason, test.SkipReason)
		})
	}
}

func TestProcessPackageOutput(t *testing.T) {
	model := &TestModel{
		packageResults: map[string]*PackageResult{
			"test/pkg": {
				Package: "test/pkg",
				Status:  "running",
				Output:  []string{},
			},
		},
		packagesWithNoTests: make(map[string]bool),
	}
	
	// Test coverage extraction
	event := &types.TestEvent{
		Package: "test/pkg",
		Output:  "coverage: 75.5% of statements",
	}
	model.processPackageOutput(event)
	assert.Equal(t, "75.5%", model.packageResults["test/pkg"].Coverage)
	
	// Test no test files detection
	event2 := &types.TestEvent{
		Package: "test/pkg",
		Output:  "?   	test/pkg	[no test files]",
	}
	model.processPackageOutput(event2)
	assert.True(t, model.packagesWithNoTests["test/pkg"])
	
	// Test failure detection
	event3 := &types.TestEvent{
		Package: "test/pkg",
		Output:  "FAIL	test/pkg	0.123s",
	}
	model.processPackageOutput(event3)
	assert.Equal(t, "fail", model.packageResults["test/pkg"].Status)
}

func TestGetBufferSizeKB(t *testing.T) {
	model := &TestModel{
		outputBuffer: strings.Repeat("a", 1024), // 1KB
		buffers: map[string][]string{
			"test1": {strings.Repeat("b", 512), strings.Repeat("c", 512)}, // 1KB
		},
		packageResults: map[string]*PackageResult{
			"pkg1": {
				Output: []string{strings.Repeat("d", 1024)}, // 1KB
				Tests:  make(map[string]*TestResult),
			},
		},
	}
	
	size := model.getBufferSizeKB()
	// Should be approximately 3KB
	assert.InDelta(t, 3.0, size, 0.1)
}