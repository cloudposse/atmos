package stream

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/gotcha/pkg/output"
)

// TestStreamReporter_ShowsCoverageInPackageSummary verifies that coverage is shown in package summaries.
func TestStreamReporter_ShowsCoverageInPackageSummary(t *testing.T) {
	// Create a buffer to capture output
	var outputBuf bytes.Buffer

	// Create a custom writer that writes to our buffer
	// This ensures we capture output regardless of GitHub Actions environment
	customWriter := output.NewCustom(&outputBuf, &outputBuf)

	reporter := NewStreamReporter(customWriter, "all", "", "standard")

	// Create a package with coverage
	pkg := &PackageResult{
		Package:   "github.com/example/pkg",
		Status:    "pass",
		HasTests:  true,
		Coverage:  "75.5%",
		Tests:     make(map[string]*TestResult),
		TestOrder: []string{"TestExample"},
	}

	pkg.Tests["TestExample"] = &TestResult{
		Name:    "TestExample",
		Status:  "pass",
		Elapsed: 0.1,
	}

	// Display the package
	reporter.OnPackageComplete(pkg)

	// Get the output
	output := outputBuf.String()

	// The package summary should include coverage
	assert.Contains(t, output, "75.5% coverage", "Should display coverage in package summary")
}

// TestStreamReporter_ShowFailedFilterShowsOnlyFailedAndSkippedTests verifies that when show="failed", only failed and skipped tests are shown.
func TestStreamReporter_ShowFailedFilterShowsOnlyFailedAndSkippedTests(t *testing.T) {
	// Create a buffer to capture output
	var outputBuf bytes.Buffer

	// Create a custom writer that writes to our buffer
	customWriter := output.NewCustom(&outputBuf, &outputBuf)

	// Create reporter with show="failed"
	reporter := NewStreamReporter(customWriter, "failed", "", "standard")

	pkg := &PackageResult{
		Package:   "test/pkg",
		Status:    "fail",
		HasTests:  true,
		Coverage:  "50.0%",
		Tests:     make(map[string]*TestResult),
		TestOrder: []string{"TestPass", "TestFail", "TestSkip"},
	}

	pkg.Tests["TestPass"] = &TestResult{
		Name:    "TestPass",
		Status:  "pass",
		Elapsed: 0.1,
	}
	pkg.Tests["TestFail"] = &TestResult{
		Name:    "TestFail",
		Status:  "fail",
		Elapsed: 0.1,
	}
	pkg.Tests["TestSkip"] = &TestResult{
		Name:    "TestSkip",
		Status:  "skip",
		Elapsed: 0.0,
	}

	reporter.OnPackageComplete(pkg)

	// Get the output
	output := outputBuf.String()

	// With show="failed", only failed and skipped tests should be shown

	// Failed tests SHOULD be shown
	assert.Contains(t, output, "TestFail", "Failed tests should be displayed with failed filter")

	// Skipped tests SHOULD be shown
	assert.Contains(t, output, "TestSkip", "Skipped tests should be displayed with failed filter")

	// BUG: Passed tests should NOT be shown, but they ARE being shown
	// This assertion will FAIL because passed tests are incorrectly displayed
	assert.NotContains(t, output, "TestPass", "Passed tests should NOT be displayed with failed filter")

	// Package summary should always be shown
	assert.Contains(t, output, "50.0% coverage", "Package summary with coverage should always be displayed")
}

// TestStreamReporter_ShowsTotalCoverageInFinalSummary verifies that total coverage appears in final summary.
func TestStreamReporter_ShowsTotalCoverageInFinalSummary(t *testing.T) {
	reporter := NewStreamReporter(nil, "all", "", "standard")

	// Capture stderr output for package processing
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Process some packages with coverage to calculate total
	pkg1 := &PackageResult{
		Package:  "test/pkg1",
		Status:   "pass",
		HasTests: true,
		Coverage: "80.0%",
		Tests: map[string]*TestResult{
			"TestSomething": {
				Name:   "TestSomething",
				Status: "pass",
			},
		},
	}
	reporter.OnPackageComplete(pkg1)

	pkg2 := &PackageResult{
		Package:  "test/pkg2",
		Status:   "pass",
		HasTests: true,
		Coverage: "60.0%",
		Tests: map[string]*TestResult{
			"TestAnother": {
				Name:   "TestAnother",
				Status: "pass",
			},
		},
	}
	reporter.OnPackageComplete(pkg2)

	// Restore stderr before calling Finalize
	os.Stderr = oldStderr
	w.Close()
	io.ReadAll(r) // Drain the pipe

	// Get final summary
	summary := reporter.Finalize(10, 2, 1, time.Second)

	// Total coverage should be shown in final summary (average of 80% and 60% = 70%)
	assert.Contains(t, summary, "Coverage", "Total coverage should be shown in final summary")
	assert.Contains(t, summary, "70.0%", "Should show average coverage of 70%")
}

// TestStreamReporter_CoverageNotShownAsStatementBreakdown verifies that coverage doesn't show statement/function breakdown.
func TestStreamReporter_CoverageNotShownAsStatementBreakdown(t *testing.T) {
	// Create a buffer to capture output
	var outputBuf bytes.Buffer

	// Create a custom writer that writes to our buffer
	customWriter := output.NewCustom(&outputBuf, &outputBuf)

	reporter := NewStreamReporter(customWriter, "all", "", "standard")

	// Create a package with coverage that should show "of statements"
	pkg := &PackageResult{
		Package:   "github.com/example/pkg",
		Status:    "pass",
		HasTests:  true,
		Coverage:  "82.3% of statements", // Input has "of statements"
		Tests:     make(map[string]*TestResult),
		TestOrder: []string{"TestExample"},
	}

	pkg.Tests["TestExample"] = &TestResult{
		Name:    "TestExample",
		Status:  "pass",
		Elapsed: 0.1,
	}

	// Display the package
	reporter.OnPackageComplete(pkg)

	// Get the output
	output := outputBuf.String()

	// The output should preserve "of statements" but currently it likely just shows "82.3% coverage"
	assert.Contains(t, output, "82.3% of statements", "BUG: Should preserve statement coverage detail but likely shows generic 'coverage'")
}
