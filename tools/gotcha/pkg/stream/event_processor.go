package stream

import (
	"strings"
	"time"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

// Status and display constants.
const (
	StatusRunning = "running"
	StatusPass    = "pass"

	// Display values.
	DisplayZeroPercent  = "0.0%"
	DisplayNotAvailable = "N/A"
)

// processEvent handles individual test events from the JSON stream.
//
//nolint:nestif,gocognit,gocyclo // The complexity here is necessary because Go's test2json output
// interleaves events from multiple packages and tests running in parallel. We must:
// - Track state for each package independently (they run concurrently)
// - Distinguish package events from test events (same JSON structure, different semantics)
// - Handle output events that may arrive before/after their associated test events
// - Parse coverage from stdout text mixed with JSON events (Go doesn't emit coverage as JSON)
// - Detect build failures from text output since Go doesn't emit them as structured events
// - Maintain parent-child relationships for subtests that may start/end in any order
// Breaking this into smaller functions would require passing extensive state between them,
// making the code harder to understand and maintain.
func (p *StreamProcessor) processEvent(event *types.TestEvent) {
	// We'll collect any package that needs to be displayed
	var packageToDisplay *PackageResult

	p.mu.Lock()

	// Handle package-level events
	if event.Test == "" {
		// Handle package start events
		switch {
		case event.Action == "start" && event.Package != "":
			// Create new package result entry
			if _, exists := p.packageResults[event.Package]; !exists {
				p.packageResults[event.Package] = &PackageResult{
					Package:   event.Package,
					StartTime: time.Now(),
					Status:    StatusRunning,
					Tests:     make(map[string]*TestResult),
					TestOrder: []string{},
					HasTests:  false,
				}
				p.packageOrder = append(p.packageOrder, event.Package)
				p.activePackages[event.Package] = true

				// Keep legacy tracking for compatibility
				p.currentPackage = event.Package
				p.packageHasTests[event.Package] = false
			}
		case event.Action == TestStatusSkip && event.Package != "" && event.Test == "":
			// Package was skipped (usually means no test files)
			if pkg, exists := p.packageResults[event.Package]; exists {
				pkg.Status = TestStatusSkip
				pkg.Elapsed = event.Elapsed
				pkg.EndTime = time.Now()
				delete(p.activePackages, event.Package)

				// Mark package for display after lock release
				packageToDisplay = pkg
			}
		case event.Action == "output" && event.Package != "" && event.Test == "":
			// Package-level output (coverage, build errors, etc.)
			if pkg, exists := p.packageResults[event.Package]; exists {
				pkg.Output = append(pkg.Output, event.Output)

				// Check for coverage
				if strings.Contains(event.Output, "coverage:") {
					// Extract coverage information properly
					switch {
					case strings.Contains(event.Output, "coverage: [no statements]"):
						// No statements to cover
						pkg.Coverage = DisplayZeroPercent
						pkg.StatementCoverage = DisplayZeroPercent
						pkg.FunctionCoverage = DisplayNotAvailable
					case strings.Contains(event.Output, "coverage: [no test files]"):
						// No test files - shouldn't happen with actual tests
						pkg.Coverage = DisplayZeroPercent
						pkg.StatementCoverage = DisplayZeroPercent
						pkg.FunctionCoverage = DisplayNotAvailable
					default:
						// Extract percentage from normal coverage output
						// Format: "coverage: 75.2% of statements"
						if matches := strings.Fields(event.Output); len(matches) >= 2 {
							for i, field := range matches {
								if field == "coverage:" && i+1 < len(matches) {
									coverage := matches[i+1]
									// Keep only valid percentage values
									if strings.HasSuffix(coverage, "%") {
										pkg.Coverage = coverage
										pkg.StatementCoverage = coverage
										// Function coverage will be calculated separately if available
										// For now, we'll use the same value or N/A
										if pkg.FunctionCoverage == "" {
											pkg.FunctionCoverage = DisplayNotAvailable
										}
									} else {
										// Handle edge cases
										pkg.Coverage = "0.0%"
										pkg.StatementCoverage = "0.0%"
										pkg.FunctionCoverage = DisplayNotAvailable
									}
									break
								}
							}
						}
					}
				}

				// Check for "no test files" message
				if strings.Contains(event.Output, "[no test files]") {
					// Mark for legacy compatibility
					p.packagesWithNoTests[event.Package] = true
				}

				// Check for package-level FAIL in output (e.g., TestMain failures)
				// This catches "FAIL\tpackage.name\t0.123s" which go test outputs
				if strings.Contains(event.Output, "FAIL\t"+event.Package) {
					// Mark package as failed - it likely has tests that failed to run
					if pkg, exists := p.packageResults[event.Package]; exists {
						// Don't override status if already set, but ensure we know tests exist
						if pkg.Status == StatusRunning {
							pkg.Status = TestStatusFail
						}
						pkg.HasTests = true // It has tests, they just failed to run
						// Store the output for display
						pkg.Output = append(pkg.Output, event.Output)
					}
				}
			}
		case event.Action == StatusPass && event.Package != "" && event.Test == "":
			// Package passed
			if pkg, exists := p.packageResults[event.Package]; exists {
				pkg.Status = StatusPass
				pkg.Elapsed = event.Elapsed
				pkg.EndTime = time.Now()
				delete(p.activePackages, event.Package)

				// Check if package had no tests
				if !pkg.HasTests || p.packagesWithNoTests[event.Package] {
					// Package has no runnable tests
					pkg.HasTests = false
				}

				// Mark package for display after lock release
				packageToDisplay = pkg
			}
		case event.Action == TestStatusFail && event.Package != "" && event.Test == "":
			// Package failed
			if pkg, exists := p.packageResults[event.Package]; exists {
				pkg.Status = TestStatusFail
				pkg.Elapsed = event.Elapsed
				pkg.EndTime = time.Now()
				delete(p.activePackages, event.Package)

				// If no tests were recorded but package failed, it likely has tests that couldn't run
				// (e.g., TestMain failure, compilation error, etc.)
				if len(pkg.Tests) == 0 && !p.packagesWithNoTests[event.Package] {
					pkg.HasTests = true
					// Count build failures as test failures in statistics
					p.failed++
				}

				// Mark package for display after lock release
				packageToDisplay = pkg
			}
		case event.Action == "output" && p.currentTest != "":
			// Package-level output might contain important command output
			// Append package-level output to the current test's buffer
			if p.buffers[p.currentTest] != nil {
				p.buffers[p.currentTest] = append(p.buffers[p.currentTest], event.Output)
			}
		}

		// Release lock and display package if needed before returning
		p.mu.Unlock()
		if packageToDisplay != nil && p.reporter != nil {
			p.reporter.OnPackageComplete(packageToDisplay)
		}
		return
	}

	// Mark that this package has tests
	if event.Package != "" && event.Test != "" {
		p.packageHasTests[event.Package] = true
		if pkg, exists := p.packageResults[event.Package]; exists {
			pkg.HasTests = true
		}
	}

	switch event.Action {
	case "run":
		p.currentTest = event.Test

		// Create test result entry in package
		if pkg, exists := p.packageResults[event.Package]; exists {
			test := &TestResult{
				Name:         event.Test,
				FullName:     event.Test,
				Status:       StatusRunning,
				Output:       []string{},
				Subtests:     make(map[string]*TestResult),
				SubtestOrder: []string{},
			}

			// Handle subtests
			if strings.Contains(event.Test, "/") {
				parts := strings.SplitN(event.Test, "/", 2)
				parentName := parts[0]
				subtestName := parts[1]

				// Ensure parent test exists in pkg.Tests and TestOrder
				parent, ok := pkg.Tests[parentName]
				if !ok {
					// Create parent test if it doesn't exist yet
					parent = &TestResult{
						Name:         parentName,
						FullName:     parentName,
						Status:       StatusRunning,
						Output:       []string{},
						Subtests:     make(map[string]*TestResult),
						SubtestOrder: []string{},
					}
					pkg.Tests[parentName] = parent
					pkg.TestOrder = append(pkg.TestOrder, parentName)
					pkg.HasTests = true
				}

				test.Parent = parentName
				test.Name = subtestName // Store just the subtest name
				parent.Subtests[event.Test] = test
				parent.SubtestOrder = append(parent.SubtestOrder, event.Test)
				// Note: We don't add subtests to pkg.Tests or pkg.TestOrder
			} else {
				// Top-level test
				pkg.Tests[event.Test] = test
				pkg.TestOrder = append(pkg.TestOrder, event.Test)
				pkg.HasTests = true
			}
		}

		// Keep legacy buffer for compatibility
		if p.buffers[event.Test] == nil {
			p.buffers[event.Test] = []string{}
		}

	case "output":
		// Buffer the output in test result
		if pkg, exists := p.packageResults[event.Package]; exists {
			if test := p.findTest(pkg, event.Test); test != nil {
				test.Output = append(test.Output, event.Output)
			}
		}

		// Keep legacy buffer for compatibility
		if p.buffers[event.Test] == nil {
			p.buffers[event.Test] = []string{}
		}
		p.buffers[event.Test] = append(p.buffers[event.Test], event.Output)

	case StatusPass:
		// Update test result
		if pkg, exists := p.packageResults[event.Package]; exists {
			if test := p.findTest(pkg, event.Test); test != nil {
				test.Status = StatusPass
				test.Elapsed = event.Elapsed
			}
		}

		// Track statistics
		p.passed++

		// Clear buffer
		delete(p.buffers, event.Test)

	case TestStatusFail:
		// Update test result
		if pkg, exists := p.packageResults[event.Package]; exists {
			if test := p.findTest(pkg, event.Test); test != nil {
				test.Status = TestStatusFail
				test.Elapsed = event.Elapsed
			}
		}

		// Track statistics
		p.failed++

		// Clear buffer
		delete(p.buffers, event.Test)

	case TestStatusSkip:
		// Update test result
		if pkg, exists := p.packageResults[event.Package]; exists {
			if test := p.findTest(pkg, event.Test); test != nil {
				test.Status = TestStatusSkip
				test.Elapsed = event.Elapsed

				// Extract skip reason from output
				for _, line := range test.Output {
					trimmedLine := strings.TrimSpace(line)

					// Look for skip reason in test output (format: "    filename.go:line: reason")
					// Example: "    skip_test.go:9: SKIP: This test is skipped for demonstration purposes"
					if strings.Contains(line, ".go:") && strings.Contains(line, ": ") {
						// Find the last colon followed by a space to get the reason
						parts := strings.SplitN(line, ": ", 2)
						if len(parts) == 2 {
							reason := strings.TrimSpace(parts[1])
							// Remove trailing newline if present
							reason = strings.TrimSuffix(reason, "\n")
							if reason != "" && test.SkipReason == "" {
								test.SkipReason = reason
								break // Found the reason, stop looking
							}
						}
					}

					// Alternative pattern: Look for t.Skip or t.Skipf calls
					if strings.Contains(trimmedLine, "t.Skip") || strings.Contains(trimmedLine, "Skipping") {
						// Extract the message after the last colon
						if idx := strings.LastIndex(line, ":"); idx >= 0 && idx < len(line)-1 {
							reason := strings.TrimSpace(line[idx+1:])
							reason = strings.TrimSuffix(reason, "\n")
							if reason != "" && test.SkipReason == "" {
								test.SkipReason = reason
								break
							}
						}
					}
				}
			}
		}

		// Track statistics
		p.skipped++

		// Clear buffer
		delete(p.buffers, event.Test)
	}

	// Release lock before doing I/O
	p.mu.Unlock()

	// Display package if needed (after releasing lock to avoid deadlock)
	if packageToDisplay != nil && p.reporter != nil {
		p.reporter.OnPackageComplete(packageToDisplay)
	}
}

// shouldShowTestStatus determines if a test with the given status should be displayed.
func (p *StreamProcessor) shouldShowTestStatus(status string) bool {
	switch p.showFilter {
	case "all":
		return true
	case "failed":
		return status == TestStatusFail
	case "passed":
		return status == StatusPass
	case "skipped":
		return status == TestStatusSkip
	case "collapsed":
		return status == TestStatusFail // Only show failures in collapsed mode
	case "none":
		return false
	default:
		return true
	}
}
