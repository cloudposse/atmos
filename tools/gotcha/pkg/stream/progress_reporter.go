package stream

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ProgressReporter implements TestReporter for TUI progress bar display.
// It buffers all output and only shows a progress bar during execution.
type ProgressReporter struct {
	mu sync.Mutex
	
	// Progress tracking
	totalTests     int
	completedTests int
	passedTests    int
	failedTests    int
	skippedTests   int
	
	// Package tracking
	totalPackages     int
	completedPackages int
	activePackage     string
	
	// Timing
	startTime time.Time
	
	// Buffered results for final display
	packageResults []*PackageResult
	
	// Configuration
	showFilter     string
	testFilter     string
	verbosityLevel string
	
	// Progress callback for UI updates
	onProgressUpdate func(completed, total int, activePackage string, elapsed time.Duration)
}

// NewProgressReporter creates a new ProgressReporter with the given configuration.
func NewProgressReporter(showFilter, testFilter, verbosityLevel string) *ProgressReporter {
	return &ProgressReporter{
		showFilter:     showFilter,
		testFilter:     testFilter,
		verbosityLevel: verbosityLevel,
		startTime:      time.Now(),
		packageResults: []*PackageResult{},
	}
}

// SetProgressCallback sets the callback function for progress updates.
func (r *ProgressReporter) SetProgressCallback(callback func(completed, total int, activePackage string, elapsed time.Duration)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onProgressUpdate = callback
}

// OnPackageStart is called when a package starts testing.
func (r *ProgressReporter) OnPackageStart(pkg *PackageResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.totalPackages++
	r.activePackage = pkg.Package
	
	// Count tests in this package for progress tracking
	testCount := 0
	for _, test := range pkg.Tests {
		testCount++
		testCount += len(test.Subtests)
	}
	r.totalTests += testCount
	
	r.notifyProgress()
}

// OnPackageComplete is called when a package finishes testing.
func (r *ProgressReporter) OnPackageComplete(pkg *PackageResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.completedPackages++
	r.packageResults = append(r.packageResults, pkg)
	
	// Count test results
	for _, test := range pkg.Tests {
		r.updateTestCounts(test)
		for _, subtest := range test.Subtests {
			r.updateTestCounts(subtest)
		}
	}
	
	// Clear active package if it was this one
	if r.activePackage == pkg.Package {
		r.activePackage = ""
	}
	
	r.notifyProgress()
}

// updateTestCounts updates the test counters based on test status.
func (r *ProgressReporter) updateTestCounts(test *TestResult) {
	r.completedTests++
	switch test.Status {
	case "pass":
		r.passedTests++
	case "fail":
		r.failedTests++
	case "skip":
		r.skippedTests++
	}
}

// OnTestStart is called when an individual test starts.
func (r *ProgressReporter) OnTestStart(pkg *PackageResult, test *TestResult) {
	// Progress reporter doesn't need to track individual test starts
	// It tracks completion for progress updates
}

// OnTestComplete is called when an individual test completes.
func (r *ProgressReporter) OnTestComplete(pkg *PackageResult, test *TestResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.updateTestCounts(test)
	r.notifyProgress()
}

// UpdateProgress updates the overall progress of test execution.
func (r *ProgressReporter) UpdateProgress(completed, total int, elapsed time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.completedTests = completed
	r.totalTests = total
	r.notifyProgress()
}

// SetEstimatedTotal sets the estimated total number of tests.
func (r *ProgressReporter) SetEstimatedTotal(total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.totalTests = total
	r.notifyProgress()
}

// notifyProgress calls the progress callback if set.
// Must be called with lock held.
func (r *ProgressReporter) notifyProgress() {
	if r.onProgressUpdate != nil {
		elapsed := time.Since(r.startTime)
		r.onProgressUpdate(r.completedTests, r.totalTests, r.activePackage, elapsed)
	}
}

// Finalize is called at the end of all test execution and returns the final output.
func (r *ProgressReporter) Finalize(passed, failed, skipped int, elapsed time.Duration) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// Build the final output string with all results
	var output strings.Builder
	
	// TODO: Display all package results like stream mode does
	// For now, just build a summary
	
	// Add final summary
	total := passed + failed + skipped
	if total > 0 {
		output.WriteString("\n\n")
		output.WriteString("Test Results:\n")
		output.WriteString(fmt.Sprintf("  ✓ Passed:  %d\n", passed))
		output.WriteString(fmt.Sprintf("  ✗ Failed:  %d\n", failed))
		output.WriteString(fmt.Sprintf("  ⊘ Skipped: %d\n", skipped))
		output.WriteString(fmt.Sprintf("  Total:     %d\n", total))
		output.WriteString("\n")
		output.WriteString(fmt.Sprintf("ℹ Tests completed in %.2fs\n", elapsed.Seconds()))
	}
	
	return output.String()
}

// GetCompletedTests returns the number of completed tests.
func (r *ProgressReporter) GetCompletedTests() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.completedTests
}

// GetTotalTests returns the total number of tests.
func (r *ProgressReporter) GetTotalTests() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.totalTests
}

// GetActivePackage returns the currently active package.
func (r *ProgressReporter) GetActivePackage() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.activePackage
}