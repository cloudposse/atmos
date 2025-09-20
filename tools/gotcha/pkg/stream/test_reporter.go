package stream

import "time"

// TestReporter defines the interface for reporting test execution progress and results.
// Different implementations can provide different display formats (e.g., stream output, progress bars).
type TestReporter interface {
	// OnPackageStart is called when a package starts testing.
	OnPackageStart(pkg *PackageResult)

	// OnPackageComplete is called when a package finishes testing.
	OnPackageComplete(pkg *PackageResult)

	// OnTestStart is called when an individual test starts.
	OnTestStart(pkg *PackageResult, test *TestResult)

	// OnTestComplete is called when an individual test completes.
	OnTestComplete(pkg *PackageResult, test *TestResult)

	// UpdateProgress updates the overall progress of test execution.
	UpdateProgress(completed, total int, elapsed time.Duration)

	// SetEstimatedTotal sets the estimated total number of tests.
	SetEstimatedTotal(total int)

	// Finalize is called at the end of all test execution and returns any final output.
	Finalize(passed, failed, skipped int, elapsed time.Duration) string
}
