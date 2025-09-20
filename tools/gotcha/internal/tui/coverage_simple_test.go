package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestTUI_ShowsTotalCoverageInFinalSummary verifies that total coverage appears in TUI final summary.
func TestTUI_ShowsTotalCoverageInFinalSummary(t *testing.T) {
	model := &TestModel{
		packageResults:    make(map[string]*PackageResult),
		packageOrder:      []string{},
		activePackages:    make(map[string]bool),
		displayedPackages: make(map[string]bool),
		showFilter:        "all",
		startTime:         time.Now(),
		passCount:         10,
		failCount:         2,
		skipCount:         1,
	}

	// Add packages with coverage
	packages := []struct {
		name     string
		coverage string
	}{
		{"pkg1", "80.0%"},
		{"pkg2", "60.0%"},
	}

	for _, p := range packages {
		pkg := &PackageResult{
			Package:  p.name,
			Status:   "pass",
			HasTests: true,
			Coverage: p.coverage,
			Tests:    make(map[string]*TestResult),
		}
		model.packageResults[p.name] = pkg
		model.packageOrder = append(model.packageOrder, p.name)
	}

	// Generate final summary
	summary := model.GenerateFinalSummary()

	// Total/average coverage should be calculated
	// Average of 80% and 60% should be 70%
	assert.Contains(t, summary, "Coverage", "Total coverage should be shown in TUI final summary")
	assert.Contains(t, summary, "70", "Average coverage (70%) should be calculated and displayed")
}

// TestTUI_ShowFailedFilterShowsOnlyFailedAndSkippedTests verifies that when show="failed", only failed and skipped tests are shown.
func TestTUI_ShowFailedFilterShowsOnlyFailedAndSkippedTests(t *testing.T) {
	model := &TestModel{
		packageResults:    make(map[string]*PackageResult),
		packageOrder:      []string{},
		activePackages:    make(map[string]bool),
		displayedPackages: make(map[string]bool),
		showFilter:        "failed", // Set filter to failed
		startTime:         time.Now(),
	}

	// Create package with mixed test results
	pkg := &PackageResult{
		Package:   "test/pkg",
		Status:    "fail",
		HasTests:  true,
		Coverage:  "65.0%",
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
		Elapsed: 0.2,
		Output:  []string{"assertion failed\n"},
	}
	pkg.Tests["TestSkip"] = &TestResult{
		Name:   "TestSkip",
		Status: "skip",
	}

	model.packageResults["test/pkg"] = pkg
	model.packageOrder = append(model.packageOrder, "test/pkg")

	// Generate display
	output := model.displayPackageResult(pkg)

	// With show="failed", only failed and skipped tests should be shown

	// Failed tests SHOULD be shown
	assert.Contains(t, output, "TestFail", "Failed tests should be displayed with failed filter")

	// Skipped tests SHOULD be shown
	assert.Contains(t, output, "TestSkip", "Skipped tests should be displayed with failed filter")

	// Passed tests should NOT be shown
	assert.NotContains(t, output, "TestPass", "Passed tests should NOT be displayed with failed filter")

	// Package summary with coverage should always be displayed
	assert.Contains(t, output, "65.0% coverage", "Package summary with coverage should always be displayed")
}
