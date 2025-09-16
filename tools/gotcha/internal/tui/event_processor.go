package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

// processEvent handles individual test events from the JSON stream.
func (m *TestModel) processEvent(event *types.TestEvent) {
	// Handle package-level events
	if event.Test == "" {
		m.processPackageEvent(event)
		return
	}

	// Handle test-level events
	m.processTestEvent(event)
}

// processPackageEvent handles package-level events.
func (m *TestModel) processPackageEvent(event *types.TestEvent) {
	switch event.Action {
	case "start":
		// New package starting
		if event.Package != "" {
			m.currentPackage = event.Package
			// Initialize package result
			if m.packageResults[event.Package] == nil {
				m.packageResults[event.Package] = &PackageResult{
					Package:   event.Package,
					StartTime: time.Now(),
					Status:    TestStatusRunning,
					Tests:     make(map[string]*TestResult),
					TestOrder: []string{},
					HasTests:  false,
				}
				m.activePackages[event.Package] = true
				// Add to packageOrder when package starts (not just when it completes)
				if !contains(m.packageOrder, event.Package) {
					m.packageOrder = append(m.packageOrder, event.Package)
					// Debug: Log package addition
					if debugFile := config.GetDebugFile(); debugFile != "" {
						if f, err := os.OpenFile(debugFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
							fmt.Fprintf(f, "[TUI-DEBUG] Added package to order from processPackageEvent: %s (total: %d)\n", event.Package, len(m.packageOrder))
							f.Close()
						}
					}
				}
			}
		}

	case "output":
		m.processPackageOutput(event)

	case "skip":
		// Package skipped (no tests to run)
		if event.Package != "" {
			if pkg := m.packageResults[event.Package]; pkg != nil {
				pkg.Status = TestStatusSkip
				pkg.EndTime = time.Now()
				pkg.Elapsed = event.Elapsed
				delete(m.activePackages, event.Package)
				if !contains(m.packageOrder, event.Package) {
					m.packageOrder = append(m.packageOrder, event.Package)
				}
			}
		}

	case "pass", "fail":
		// Package completed
		if event.Package != "" {
			if pkg := m.packageResults[event.Package]; pkg != nil {
				pkg.Status = event.Action
				pkg.EndTime = time.Now()
				pkg.Elapsed = event.Elapsed
				delete(m.activePackages, event.Package)
				if !contains(m.packageOrder, event.Package) {
					m.packageOrder = append(m.packageOrder, event.Package)
				}

				// If package failed with no tests recorded, it likely has tests that couldn't run
				// (e.g., TestMain failure, compilation error, etc.)
				if event.Action == "fail" && len(pkg.Tests) == 0 && !m.packagesWithNoTests[event.Package] {
					pkg.HasTests = true
				}
			}
		}
	}
}

// processPackageOutput handles package-level output events.
func (m *TestModel) processPackageOutput(event *types.TestEvent) {
	if event.Package == "" {
		return
	}

	// Check for coverage or "no test files" message
	if strings.Contains(event.Output, "coverage:") {
		m.extractCoverage(event)
	}

	if strings.Contains(event.Output, "[no test files]") {
		m.packagesWithNoTests[event.Package] = true
	}

	// Check for package-level FAIL in output (e.g., TestMain failures)
	// This catches "FAIL\tpackage.name\t0.123s" which go test outputs
	if strings.Contains(event.Output, "FAIL\t"+event.Package) {
		// Mark package as failed - it likely has tests that failed to run
		if pkg := m.packageResults[event.Package]; pkg != nil {
			// Don't override status if already set, but ensure we know tests exist
			if pkg.Status == TestStatusRunning {
				pkg.Status = TestStatusFail
			}
			pkg.HasTests = true // It has tests, they just failed to run
		}
	}

	// Buffer package-level output
	if pkg := m.packageResults[event.Package]; pkg != nil {
		pkg.Output = append(pkg.Output, event.Output)
	}
}

// extractCoverage extracts coverage information from output.
func (m *TestModel) extractCoverage(event *types.TestEvent) {
	switch {
	case strings.Contains(event.Output, "coverage: [no statements]"):
		// No statements to cover
		m.packageResults[event.Package].Coverage = "0.0%"
	case strings.Contains(event.Output, "coverage: [no test files]"):
		// No test files - shouldn't happen with actual tests
		m.packageResults[event.Package].Coverage = "0.0%"
	default:
		// Extract percentage from normal coverage output
		if matches := strings.Fields(event.Output); len(matches) >= 2 {
			for i, field := range matches {
				if field == "coverage:" && i+1 < len(matches) {
					coverage := matches[i+1]
					// Remove any trailing characters
					if strings.HasSuffix(coverage, "%") {
						m.packageResults[event.Package].Coverage = coverage
					} else {
						// Handle edge cases
						m.packageResults[event.Package].Coverage = "0.0%"
					}
					break
				}
			}
		}
	}
}

// processTestEvent handles test-level events.
func (m *TestModel) processTestEvent(event *types.TestEvent) {
	if event.Package == "" || event.Test == "" {
		return
	}

	pkg := m.packageResults[event.Package]
	if pkg == nil {
		// Create package if it doesn't exist (can happen with out-of-order events)
		pkg = &PackageResult{
			Package:   event.Package,
			StartTime: time.Now(),
			Status:    TestStatusRunning,
			Tests:     make(map[string]*TestResult),
			TestOrder: []string{},
			HasTests:  false,
		}
		m.packageResults[event.Package] = pkg
		m.activePackages[event.Package] = true
		// Add to packageOrder when package is created
		if !contains(m.packageOrder, event.Package) {
			m.packageOrder = append(m.packageOrder, event.Package)
			// Debug: Log package addition from test event
			if debugFile := config.GetDebugFile(); debugFile != "" {
				if f, err := os.OpenFile(debugFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
					fmt.Fprintf(f, "[TUI-DEBUG] Added package to order from processTestEvent: %s (total: %d)\n", event.Package, len(m.packageOrder))
					f.Close()
				}
			}
		}
	}

	// Mark that this package has tests
	pkg.HasTests = true

	// Parse test hierarchy
	var parentTest string
	var isSubtest bool
	if strings.Contains(event.Test, "/") {
		parts := strings.SplitN(event.Test, "/", 2)
		parentTest = parts[0]
		isSubtest = true
	}

	switch event.Action {
	case "run":
		m.processTestRun(event, pkg, parentTest, isSubtest)

	case "output":
		m.processTestOutput(event, pkg, parentTest, isSubtest)

	case "pass", "fail", "skip":
		m.processTestResult(event, pkg, parentTest, isSubtest)
	}
}

// processTestRun handles test run events.
func (m *TestModel) processTestRun(event *types.TestEvent, pkg *PackageResult, parentTest string, isSubtest bool) {
	// Debug: Log all test run events
	if debugFile := config.GetDebugFile(); debugFile != "" {
		if f, err := os.OpenFile(debugFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			fmt.Fprintf(f, "[RUN-DEBUG] Test run event for package %s: %s (isSubtest: %v)\n",
				event.Package, event.Test, isSubtest)
			f.Close()
		}
	}

	m.currentTest = event.Test
	// Count all tests including subtests for accurate progress
	// Always increment the actual test count
	m.actualTestCount++

	if !m.usingEstimate {
		// Not using estimate, update totalTests with actual count
		m.totalTests = m.actualTestCount
	}
	// If using estimate, keep totalTests as the estimate value

	if isSubtest {
		// This is a subtest
		parent := pkg.Tests[parentTest]
		if parent == nil {
			// Parent test might not exist yet due to parallel execution or table-driven tests
			parent = &TestResult{
				Name:         parentTest,
				FullName:     parentTest,
				Status:       TestStatusRunning,
				Subtests:     make(map[string]*TestResult),
				SubtestOrder: []string{},
			}
			pkg.Tests[parentTest] = parent

			// Also add the parent to TestOrder if it's not already there
			// This ensures parent appears before its subtests in display
			if !contains(pkg.TestOrder, parentTest) {
				pkg.TestOrder = append(pkg.TestOrder, parentTest)
			}
		}

		subtest := &TestResult{
			Name:     event.Test,
			FullName: event.Test,
			Status:   TestStatusRunning,
			Parent:   parentTest,
		}
		parent.Subtests[event.Test] = subtest
		parent.SubtestOrder = append(parent.SubtestOrder, event.Test)

		// IMPORTANT: Add subtest to both pkg.Tests AND pkg.TestOrder for display
		// This ensures subtests are accessible and visible in the TUI output
		pkg.Tests[event.Test] = subtest
		pkg.TestOrder = append(pkg.TestOrder, event.Test)
	} else {
		// Top-level test
		test := &TestResult{
			Name:         event.Test,
			FullName:     event.Test,
			Status:       TestStatusRunning,
			Subtests:     make(map[string]*TestResult),
			SubtestOrder: []string{},
		}
		pkg.Tests[event.Test] = test
		pkg.TestOrder = append(pkg.TestOrder, event.Test)

		// Debug: Log when we add a test to TestOrder
		if debugFile := config.GetDebugFile(); debugFile != "" {
			if f, err := os.OpenFile(debugFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
				fmt.Fprintf(f, "[EVENT-DEBUG] Added test to TestOrder for package %s: %s (total in order: %d)\n",
					event.Package, event.Test, len(pkg.TestOrder))
				f.Close()
			}
		}
	}
}

// processTestOutput handles test output events.
func (m *TestModel) processTestOutput(event *types.TestEvent, pkg *PackageResult, parentTest string, isSubtest bool) {
	// Buffer the output
	if isSubtest {
		if parent := pkg.Tests[parentTest]; parent != nil {
			if subtest := parent.Subtests[event.Test]; subtest != nil {
				subtest.Output = append(subtest.Output, event.Output)
				// Capture skip reason if this is a skip output
				m.extractSkipReason(event.Output, subtest)
			}
		}
	} else {
		if test := pkg.Tests[event.Test]; test != nil {
			test.Output = append(test.Output, event.Output)
			// Capture skip reason if this is a skip output
			m.extractSkipReason(event.Output, test)
		}
	}
}

// extractSkipReason extracts skip reason from output.
func (m *TestModel) extractSkipReason(output string, test *TestResult) {
	// Skip lines like "--- SKIP: TestName (0.00s)"
	if strings.HasPrefix(strings.TrimSpace(output), "---") {
		return
	}

	// Check if this is a skip output
	if !strings.Contains(output, "SKIP:") && !strings.Contains(output, "SKIP ") &&
		!strings.Contains(output, "skipping:") && !strings.Contains(output, "Skipping ") &&
		!strings.Contains(output, "Skip(") && !strings.Contains(output, "Skipf(") {
		return
	}

	// Extract just the reason part, not the full output
	reason := strings.TrimSpace(output)

	// Remove file:line: prefix if present (e.g., "skip_test.go:9: ")
	if idx := strings.LastIndex(reason, ": "); idx >= 0 && idx < len(reason)-2 {
		// Check if this looks like a file:line prefix
		prefix := reason[:idx]
		if strings.Contains(prefix, ".go:") || strings.Contains(prefix, "_test.go:") {
			reason = strings.TrimSpace(reason[idx+2:])
		}
	}

	// Now extract the actual skip message
	if idx := strings.Index(reason, "SKIP:"); idx >= 0 {
		reason = strings.TrimSpace(reason[idx+5:])
	} else if idx := strings.Index(reason, "SKIP "); idx >= 0 {
		reason = strings.TrimSpace(reason[idx+5:])
	} else if idx := strings.Index(reason, "skipping:"); idx >= 0 {
		reason = strings.TrimSpace(reason[idx+9:])
	} else if idx := strings.Index(reason, "Skipping "); idx >= 0 {
		reason = strings.TrimSpace(reason[idx+9:])
	}

	// Only set if we have a meaningful reason
	if reason != "" && !strings.HasPrefix(reason, "---") {
		test.SkipReason = reason

		// Log to file for debugging
		if f, err := os.OpenFile("/tmp/gotcha-debug.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			fmt.Fprintf(f, "[DEBUG] Captured skip reason for %s: %q\n", test.Name, reason)
			f.Close()
		}
	}
}

// processTestResult handles test result events (pass, fail, skip).
func (m *TestModel) processTestResult(event *types.TestEvent, pkg *PackageResult, parentTest string, isSubtest bool) {
	// Count all tests including subtests for accurate progress
	m.completedTests++

	// Check if we should switch from estimate to actual count
	if m.usingEstimate && m.actualTestCount > 0 {
		// Only switch from estimate to actual when we're confident:
		// 1. If actual count exceeds the estimate (estimate was too low)
		// 2. If we've completed a significant portion of the estimated tests
		if m.actualTestCount > m.estimatedTestCount {
			// Actual count exceeded estimate, switch to actual
			m.usingEstimate = false
			m.totalTests = m.actualTestCount
		} else if m.completedTests > int(float64(m.estimatedTestCount)*0.9) {
			// We've completed 90% of estimated tests, likely near the end
			// Switch to actual count for accuracy
			m.usingEstimate = false
			m.totalTests = m.actualTestCount
		}
		// Otherwise keep showing the estimate to avoid jarring updates
	}

	// Update counts
	switch event.Action {
	case "pass":
		m.passCount++
	case "fail":
		m.failCount++
	case "skip":
		m.skipCount++
	}

	// Update test result
	if isSubtest {
		if parent := pkg.Tests[parentTest]; parent != nil {
			if subtest := parent.Subtests[event.Test]; subtest != nil {
				subtest.Status = event.Action
				subtest.Elapsed = event.Elapsed
			}
			// Update parent subtest stats
			if m.subtestStats[parentTest] == nil {
				m.subtestStats[parentTest] = &SubtestStats{}
			}
			switch event.Action {
			case "pass":
				m.subtestStats[parentTest].passed = append(m.subtestStats[parentTest].passed, event.Test)
			case "fail":
				m.subtestStats[parentTest].failed = append(m.subtestStats[parentTest].failed, event.Test)
			case "skip":
				m.subtestStats[parentTest].skipped = append(m.subtestStats[parentTest].skipped, event.Test)
			}
		}
	} else {
		if test := pkg.Tests[event.Test]; test != nil {
			test.Status = event.Action
			test.Elapsed = event.Elapsed
			// Skip reason is already captured from output events
		}
	}
}
