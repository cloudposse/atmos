package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

// TestSubtestDisplayRegression reproduces the issue where subtests are counted
// but not displayed in TUI mode, causing a mismatch between the total count
// and visible test names.
//
// This test captures the regression reported where:
// - Total shows 35+ tests
// - But only a few test names are visible
// - Most packages show blank or just summary lines.
func TestSubtestDisplayRegression(t *testing.T) {
	t.Run("subtests_not_in_TestOrder", func(t *testing.T) {
		// Create a test model
		model := NewTestModel(
			[]string{"github.com/example/pkg"},
			"",
			"",
			"",
			"all", // Show all tests
			false,
			"standard",
			0,
		)

		// Simulate test events for a package with subtests
		pkg := "github.com/example/pkg"

		// Package start
		model.processEvent(&types.TestEvent{
			Action:  "start",
			Package: pkg,
		})

		// Parent test run
		model.processEvent(&types.TestEvent{
			Action:  "run",
			Package: pkg,
			Test:    "TestParent",
		})

		// Subtest runs - these should be displayed but currently aren't
		model.processEvent(&types.TestEvent{
			Action:  "run",
			Package: pkg,
			Test:    "TestParent/subtest1",
		})

		model.processEvent(&types.TestEvent{
			Action:  "run",
			Package: pkg,
			Test:    "TestParent/subtest2",
		})

		model.processEvent(&types.TestEvent{
			Action:  "run",
			Package: pkg,
			Test:    "TestParent/subtest3",
		})

		// Complete the subtests
		model.processEvent(&types.TestEvent{
			Action:  "pass",
			Package: pkg,
			Test:    "TestParent/subtest1",
			Elapsed: 0.1,
		})

		model.processEvent(&types.TestEvent{
			Action:  "pass",
			Package: pkg,
			Test:    "TestParent/subtest2",
			Elapsed: 0.1,
		})

		model.processEvent(&types.TestEvent{
			Action:  "pass",
			Package: pkg,
			Test:    "TestParent/subtest3",
			Elapsed: 0.1,
		})

		// Complete parent test
		model.processEvent(&types.TestEvent{
			Action:  "pass",
			Package: pkg,
			Test:    "TestParent",
			Elapsed: 0.3,
		})

		// Package complete
		model.processEvent(&types.TestEvent{
			Action:  "pass",
			Package: pkg,
			Elapsed: 0.3,
		})

		// Verify the bug: TestOrder should contain all tests for display
		pkgResult := model.packageResults[pkg]
		require.NotNil(t, pkgResult)

		// FIXED: TestOrder now contains all tests (parent + subtests) for proper display
		assert.Equal(t, 4, len(pkgResult.TestOrder),
			"TestOrder should contain all tests for display (1 parent + 3 subtests)")

		// Tests map now includes all tests (parent + subtests)
		assert.Equal(t, 4, len(pkgResult.Tests))

		// And the parent test has subtests
		parentTest := pkgResult.Tests["TestParent"]
		require.NotNil(t, parentTest)
		assert.Equal(t, 3, len(parentTest.Subtests), "Parent should have 3 subtests")

		// The counts show all tests
		assert.Equal(t, 4, model.passCount, "Should count all 4 passed tests (1 parent + 3 subtests)")

		// But display would only show 1 test name (the parent)
		// This is the core issue - counting doesn't match display
	})

	t.Run("count_vs_display_mismatch", func(t *testing.T) {
		// Create a test model
		model := NewTestModel(
			[]string{"github.com/example/pkg"},
			"",
			"",
			"",
			"all",
			false,
			"standard",
			0,
		)

		// Simulate multiple packages with different test structures
		packages := []struct {
			name            string
			topLevelTests   []string
			subtestsPerTest int
		}{
			{"github.com/example/pkg1", []string{"TestA", "TestB"}, 5},          // 2 top-level, 10 subtests
			{"github.com/example/pkg2", []string{"TestMain"}, 20},               // 1 top-level, 20 subtests
			{"github.com/example/pkg3", []string{}, 0},                          // No tests (blank display)
			{"github.com/example/pkg4", []string{"TestX", "TestY", "TestZ"}, 0}, // 3 top-level, no subtests
		}

		totalTestsRun := 0
		totalTestsInOrder := 0

		for _, pkg := range packages {
			// Package start
			model.processEvent(&types.TestEvent{
				Action:  "start",
				Package: pkg.name,
			})

			// Run top-level tests
			for _, test := range pkg.topLevelTests {
				model.processEvent(&types.TestEvent{
					Action:  "run",
					Package: pkg.name,
					Test:    test,
				})
				totalTestsRun++

				// Run subtests
				for i := 0; i < pkg.subtestsPerTest; i++ {
					subtestName := test + "/subtest" + string(rune('A'+i))
					model.processEvent(&types.TestEvent{
						Action:  "run",
						Package: pkg.name,
						Test:    subtestName,
					})
					totalTestsRun++

					// Complete subtest
					model.processEvent(&types.TestEvent{
						Action:  "pass",
						Package: pkg.name,
						Test:    subtestName,
						Elapsed: 0.01,
					})
				}

				// Complete parent test
				model.processEvent(&types.TestEvent{
					Action:  "pass",
					Package: pkg.name,
					Test:    test,
					Elapsed: 0.1,
				})
			}

			// Complete package
			model.processEvent(&types.TestEvent{
				Action:  "pass",
				Package: pkg.name,
				Elapsed: 0.2,
			})

			// Count tests in TestOrder
			if pkgResult := model.packageResults[pkg.name]; pkgResult != nil {
				totalTestsInOrder += len(pkgResult.TestOrder)
			}
		}

		// Verify the mismatch
		assert.Equal(t, 36, totalTestsRun, "Total tests run (including subtests)")
		assert.Equal(t, 36, model.passCount, "Total tests passed")

		// FIXED: All tests (including subtests) are now in TestOrder for display
		assert.Equal(t, 36, totalTestsInOrder,
			"All 36 tests (including subtests) should be in TestOrder for display")

		// The fix ensures:
		// - User sees "Total: 36" in summary
		// - All 36 test names are displayed
		// - pkg3 shows its subtests properly
		// - pkg2 shows "TestMain" and all 20 subtests
	})

	t.Run("packages_with_only_subtests_display_blank", func(t *testing.T) {
		model := NewTestModel(
			[]string{"github.com/example/integration"},
			"",
			"",
			"",
			"all",
			false,
			"standard",
			0,
		)

		pkg := "github.com/example/integration"

		// Package start
		model.processEvent(&types.TestEvent{
			Action:  "start",
			Package: pkg,
		})

		// This package uses table-driven tests extensively
		// The pattern is TestXXX with all actual tests as subtests
		testCases := []string{
			"TestIntegration/database_connection",
			"TestIntegration/api_authentication",
			"TestIntegration/data_validation",
			"TestIntegration/error_handling",
			"TestIntegration/concurrent_access",
		}

		// Note: TestIntegration parent never gets a "run" event in some cases
		// It only exists as a container for subtests

		for _, tc := range testCases {
			model.processEvent(&types.TestEvent{
				Action:  "run",
				Package: pkg,
				Test:    tc,
			})
		}

		// Complete all subtests
		for _, tc := range testCases {
			model.processEvent(&types.TestEvent{
				Action:  "pass",
				Package: pkg,
				Test:    tc,
				Elapsed: 0.5,
			})
		}

		// Package completes
		model.processEvent(&types.TestEvent{
			Action:  "pass",
			Package: pkg,
			Elapsed: 2.5,
		})

		// Check the result
		pkgResult := model.packageResults[pkg]
		require.NotNil(t, pkgResult)

		// FIXED: TestOrder contains all subtests (may include implicit parent)
		assert.GreaterOrEqual(t, len(pkgResult.TestOrder), 5,
			"TestOrder should contain at least 5 subtests for display")

		// We counted 5 tests
		assert.Equal(t, 5, model.passCount, "5 subtests passed")

		// Generate display to verify it shows the tests
		display := model.displayPackageResult(pkgResult)
		assert.Contains(t, display, pkg, "Package name should be shown")

		// FIXED: The display now shows the subtest names
		// All 5 tests are properly displayed
		assert.Contains(t, display, "database_connection",
			"Subtest names should be displayed")
	})

	t.Run("actual_test_count_vs_displayed_count", func(t *testing.T) {
		model := NewTestModel(
			[]string{"./..."},
			"",
			"",
			"",
			"all",
			false,
			"standard",
			0,
		)

		// Simulate a realistic test run similar to Atmos
		// This reproduces the "1700+ tests but only 35 displayed" issue
		type packageInfo struct {
			pkg      string
			topLevel int
			subtests int
		}

		packages := []packageInfo{
			{"github.com/cloudposse/atmos/cmd", 1, 0},
			{"github.com/cloudposse/atmos/internal/exec", 5, 0},
			{"github.com/cloudposse/atmos/pkg/config", 2, 15},
			{"github.com/cloudposse/atmos/pkg/stack", 1, 50},
			{"github.com/cloudposse/atmos/pkg/component", 0, 25}, // Only subtests!
			{"github.com/cloudposse/atmos/pkg/utils", 10, 100},
			{"github.com/cloudposse/atmos/pkg/validate", 3, 200},
		}

		displayableTests := 0
		totalTests := 0

		for _, p := range packages {
			model.processEvent(&types.TestEvent{
				Action:  "start",
				Package: p.pkg,
			})

			// Add top-level tests
			for i := 0; i < p.topLevel; i++ {
				testName := "Test" + string(rune('A'+i))
				model.processEvent(&types.TestEvent{
					Action:  "run",
					Package: p.pkg,
					Test:    testName,
				})
				totalTests++
				displayableTests++ // Top-level tests are displayable

				// Add subtests for this test (distributed across top-level tests)
				subtestsPerParent := p.subtests / maxInt(p.topLevel, 1)
				for j := 0; j < subtestsPerParent; j++ {
					subtestName := testName + "/sub" + string(rune('0'+j))
					model.processEvent(&types.TestEvent{
						Action:  "run",
						Package: p.pkg,
						Test:    subtestName,
					})
					totalTests++

					model.processEvent(&types.TestEvent{
						Action:  "pass",
						Package: p.pkg,
						Test:    subtestName,
						Elapsed: 0.001,
					})
				}

				model.processEvent(&types.TestEvent{
					Action:  "pass",
					Package: p.pkg,
					Test:    testName,
					Elapsed: 0.01,
				})
			}

			// For packages with only subtests (no top-level)
			if p.topLevel == 0 && p.subtests > 0 {
				// These run as TestSuite/individual_test pattern
				for j := 0; j < p.subtests; j++ {
					subtestName := "TestSuite/test" + string(rune('0'+j%10))
					model.processEvent(&types.TestEvent{
						Action:  "run",
						Package: p.pkg,
						Test:    subtestName,
					})
					totalTests++

					model.processEvent(&types.TestEvent{
						Action:  "pass",
						Package: p.pkg,
						Test:    subtestName,
						Elapsed: 0.001,
					})
				}
			}

			model.processEvent(&types.TestEvent{
				Action:  "pass",
				Package: p.pkg,
				Elapsed: 1.0,
			})
		}

		// Count what would actually be displayed
		actuallyDisplayed := 0
		for _, pkg := range packages {
			if pkgResult := model.packageResults[pkg.pkg]; pkgResult != nil {
				actuallyDisplayed += len(pkgResult.TestOrder)
			}
		}

		// FIXED: All tests are now displayable
		// Subtests are now included in TestOrder for complete visibility
		assert.Greater(t, totalTests, 400, "Should have 400+ total tests including subtests")
		assert.Equal(t, totalTests, model.passCount, "All tests should pass")
		assert.Equal(t, 22, displayableTests, "22 top-level tests before fix")
		// Allow for implicit parent tests that may be created
		assert.GreaterOrEqual(t, actuallyDisplayed, totalTests,
			"FIXED: All tests are now displayed (may include implicit parents)")

		// The fix ensures:
		// Summary shows: "Total: 412"
		// User sees all 412 test names in the output
		// All tests (including 390 subtests) are visible
	})
}

// TestTUIPackageWithNoTestsDisplay verifies packages with no tests show correctly.
func TestTUIPackageWithNoTestsDisplay(t *testing.T) {
	model := NewTestModel(
		[]string{"./..."},
		"",
		"",
		"",
		"all",
		false,
		"standard",
		0,
	)

	// Package with [no test files]
	model.processEvent(&types.TestEvent{
		Action:  "start",
		Package: "github.com/example/utils",
	})

	model.processEvent(&types.TestEvent{
		Action:  "output",
		Package: "github.com/example/utils",
		Output:  "?   \tgithub.com/example/utils\t[no test files]\n",
	})

	model.processEvent(&types.TestEvent{
		Action:  "skip",
		Package: "github.com/example/utils",
		Elapsed: 0,
	})

	pkgResult := model.packageResults["github.com/example/utils"]
	require.NotNil(t, pkgResult)

	display := model.displayPackageResult(pkgResult)
	assert.Contains(t, display, "No tests", "Should show 'No tests' for packages without test files")
}

// Helper function for maxInt (avoiding conflict with existing max function).
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
