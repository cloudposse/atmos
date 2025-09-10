package stream

import (
	"strings"
	"time"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

// processEvent handles individual test events from the JSON stream.
func (p *StreamProcessor) processEvent(event *types.TestEvent) {
	// We'll collect any package that needs to be displayed
	var packageToDisplay *PackageResult

	p.mu.Lock()

	// Handle package-level events
	if event.Test == "" {
		// Handle package start events
		if event.Action == "start" && event.Package != "" {
			// Create new package result entry
			if _, exists := p.packageResults[event.Package]; !exists {
				p.packageResults[event.Package] = &PackageResult{
					Package:   event.Package,
					StartTime: time.Now(),
					Status:    "running",
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
		} else if event.Action == "skip" && event.Package != "" && event.Test == "" {
			// Package was skipped (usually means no test files)
			if pkg, exists := p.packageResults[event.Package]; exists {
				pkg.Status = "skip"
				pkg.Elapsed = event.Elapsed
				pkg.EndTime = time.Now()
				delete(p.activePackages, event.Package)

				// Mark package for display after lock release
				packageToDisplay = pkg
			}
		} else if event.Action == "output" && event.Package != "" && event.Test == "" {
			// Package-level output (coverage, build errors, etc.)
			if pkg, exists := p.packageResults[event.Package]; exists {
				pkg.Output = append(pkg.Output, event.Output)

				// Check for coverage
				if strings.Contains(event.Output, "coverage:") {
					// Extract coverage information properly
					if strings.Contains(event.Output, "coverage: [no statements]") {
						// No statements to cover
						pkg.Coverage = "0.0%"
						pkg.StatementCoverage = "0.0%"
						pkg.FunctionCoverage = "N/A"
					} else if strings.Contains(event.Output, "coverage: [no test files]") {
						// No test files - shouldn't happen with actual tests
						pkg.Coverage = "0.0%"
						pkg.StatementCoverage = "0.0%"
						pkg.FunctionCoverage = "N/A"
					} else {
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
											pkg.FunctionCoverage = "N/A"
										}
									} else {
										// Handle edge cases
										pkg.Coverage = "0.0%"
										pkg.StatementCoverage = "0.0%"
										pkg.FunctionCoverage = "N/A"
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
						if pkg.Status == "running" {
							pkg.Status = "fail"
						}
						pkg.HasTests = true // It has tests, they just failed to run
						// Store the output for display
						pkg.Output = append(pkg.Output, event.Output)
					}
				}
			}
		} else if event.Action == "pass" && event.Package != "" && event.Test == "" {
			// Package passed
			if pkg, exists := p.packageResults[event.Package]; exists {
				pkg.Status = "pass"
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
		} else if event.Action == "fail" && event.Package != "" && event.Test == "" {
			// Package failed
			if pkg, exists := p.packageResults[event.Package]; exists {
				pkg.Status = "fail"
				pkg.Elapsed = event.Elapsed
				pkg.EndTime = time.Now()
				delete(p.activePackages, event.Package)

				// If no tests were recorded but package failed, it likely has tests that couldn't run
				// (e.g., TestMain failure, compilation error, etc.)
				if len(pkg.Tests) == 0 && !p.packagesWithNoTests[event.Package] {
					pkg.HasTests = true
				}

				// Mark package for display after lock release
				packageToDisplay = pkg
			}
		} else if event.Action == "output" && p.currentTest != "" {
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
				Status:       "running",
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
						Status:       "running",
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

	case "pass":
		// Update test result
		if pkg, exists := p.packageResults[event.Package]; exists {
			if test := p.findTest(pkg, event.Test); test != nil {
				test.Status = "pass"
				test.Elapsed = event.Elapsed
			}
		}

		// Track statistics
		p.passed++

		// Clear buffer
		delete(p.buffers, event.Test)

	case "fail":
		// Update test result
		if pkg, exists := p.packageResults[event.Package]; exists {
			if test := p.findTest(pkg, event.Test); test != nil {
				test.Status = "fail"
				test.Elapsed = event.Elapsed
			}
		}

		// Track statistics
		p.failed++

		// Clear buffer
		delete(p.buffers, event.Test)

	case "skip":
		// Update test result
		if pkg, exists := p.packageResults[event.Package]; exists {
			if test := p.findTest(pkg, event.Test); test != nil {
				test.Status = "skip"
				test.Elapsed = event.Elapsed
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

// shouldShowTestEvent determines if a test event should be displayed based on filter.
func (p *StreamProcessor) shouldShowTestEvent(action string) bool {
	switch p.showFilter {
	case "all":
		return true
	case "failed":
		return action == "fail"
	case "passed":
		return action == "pass"
	case "skipped":
		return action == "skip"
	case "collapsed", "none":
		return false
	default:
		return true
	}
}

// shouldShowTestStatus determines if a test with the given status should be displayed.
func (p *StreamProcessor) shouldShowTestStatus(status string) bool {
	switch p.showFilter {
	case "all":
		return true
	case "failed":
		return status == "fail"
	case "passed":
		return status == "pass"
	case "skipped":
		return status == "skip"
	case "collapsed":
		return status == "fail" // Only show failures in collapsed mode
	case "none":
		return false
	default:
		return true
	}
}
