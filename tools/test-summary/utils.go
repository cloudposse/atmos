package main

import (
	"sort"
	"strings"
)

// ShortPackage shortens a package name for readability.
// Example: github.com/cloudposse/atmos/cmd -> cmd.
func shortPackage(pkg string) string {
	parts := strings.Split(pkg, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return pkg
}

// filterTestsByPackages returns tests that belong to the specified packages.
func filterTestsByPackages(tests []TestResult, packages []string) []TestResult {
	if len(packages) == 0 {
		return []TestResult{} // No changed packages, return empty.
	}

	packageSet := make(map[string]bool)
	for _, pkg := range packages {
		packageSet[pkg] = true
	}

	var filtered []TestResult
	for _, test := range tests {
		// Check if test package ends with any of the changed packages.
		for pkg := range packageSet {
			if strings.Contains(test.Package, pkg) {
				filtered = append(filtered, test)
				break
			}
		}
	}
	return filtered
}

// getTopSlowestTests returns the N slowest tests.
func getTopSlowestTests(tests []TestResult, n int) []TestResult {
	if len(tests) <= n {
		return tests
	}

	// Sort by duration descending.
	sorted := make([]TestResult, len(tests))
	copy(sorted, tests)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Duration > sorted[j].Duration
	})

	return sorted[:n]
}

// generatePackageSummary creates package-level statistics.
func generatePackageSummary(tests []TestResult) []PackageSummary {
	packageStats := make(map[string]*PackageSummary)

	for _, test := range tests {
		pkg := shortPackage(test.Package)
		if _, exists := packageStats[pkg]; !exists {
			packageStats[pkg] = &PackageSummary{
				Package: pkg,
			}
		}
		stats := packageStats[pkg]
		stats.TestCount++
		stats.TotalDuration += test.Duration
	}

	var summaries []PackageSummary
	for _, stats := range packageStats {
		stats.AvgDuration = stats.TotalDuration / float64(stats.TestCount)
		summaries = append(summaries, *stats)
	}

	// Sort by total duration descending.
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].TotalDuration > summaries[j].TotalDuration
	})

	return summaries
}
