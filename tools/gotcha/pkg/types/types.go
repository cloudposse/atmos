package types

import (
	"strings"
	"time"
)

// TestEvent represents a single event from go test -json output.
type TestEvent struct {
	Time    time.Time `json:"Time"`
	Action  string    `json:"Action"`
	Package string    `json:"Package"`
	Test    string    `json:"Test"`
	Output  string    `json:"Output"`
	Elapsed float64   `json:"Elapsed"`
}

// TestResult represents the final result of a single test.
type TestResult struct {
	Package    string
	Test       string
	Status     string
	Duration   float64
	SkipReason string // Reason why test was skipped (if applicable)
}

// BuildFailure represents a package that failed to build.
type BuildFailure struct {
	Package string
	Output  string // Build error output if available
}

// TestSummary represents the overall summary of test results.
type TestSummary struct {
	Failed           []TestResult
	Skipped          []TestResult
	Passed           []TestResult
	BuildFailed      []BuildFailure // Build failures (packages that failed to compile)
	Coverage         string
	CoverageData     *CoverageData
	TotalElapsedTime float64 // Total elapsed time in seconds
}

// CoverageFunction represents a function's coverage information.
type CoverageFunction struct {
	File     string
	Function string
	Coverage float64
}

// CoverageData contains detailed coverage information.
type CoverageData struct {
	StatementCoverage string
	FunctionCoverage  []CoverageFunction
	FilteredFiles     []string // Files excluded from coverage.
}

// PackageSummary represents test statistics for a package.
type PackageSummary struct {
	Package       string
	TestCount     int
	AvgDuration   float64
	TotalDuration float64
}

// CoverageLine represents parsed coverage data from a single line.
type CoverageLine struct {
	Filename   string
	Statements int
	Covered    int
}

// ShortPackage returns the last part of a package path.
func ShortPackage(pkg string) string {
	parts := strings.Split(pkg, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return pkg
}
