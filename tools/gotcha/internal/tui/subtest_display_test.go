package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

// TestSubtestsAreInTestOrder verifies that subtests are included in TestOrder
// for display purposes. This test SHOULD FAIL until the bug is fixed.
func TestSubtestsAreInTestOrder(t *testing.T) {
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

	pkg := "github.com/example/pkg"
	
	// Simulate test events
	model.processEvent(&types.TestEvent{
		Action:  "start",
		Package: pkg,
	})

	// Parent test
	model.processEvent(&types.TestEvent{
		Action:  "run",
		Package: pkg,
		Test:    "TestParent",
	})

	// Subtests
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

	// Complete subtests
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

	// Complete parent
	model.processEvent(&types.TestEvent{
		Action:  "pass",
		Package: pkg,
		Test:    "TestParent",
		Elapsed: 0.3,
	})

	model.processEvent(&types.TestEvent{
		Action:  "pass",
		Package: pkg,
		Elapsed: 0.3,
	})

	pkgResult := model.packageResults[pkg]
	require.NotNil(t, pkgResult)

	// THIS TEST SHOULD FAIL UNTIL BUG IS FIXED
	// TestOrder should contain ALL tests for proper display
	assert.Equal(t, 4, len(pkgResult.TestOrder), 
		"TestOrder should contain all 4 tests (1 parent + 3 subtests) for display")
	
	// Verify all test names are in TestOrder
	expectedTests := []string{
		"TestParent",
		"TestParent/subtest1", 
		"TestParent/subtest2",
		"TestParent/subtest3",
	}
	
	for _, expected := range expectedTests {
		assert.Contains(t, pkgResult.TestOrder, expected,
			"TestOrder should contain %s", expected)
	}
}

// TestCountMatchesDisplay verifies that the total count matches the number
// of displayed test names. This test SHOULD FAIL until the bug is fixed.
func TestCountMatchesDisplay(t *testing.T) {
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

	// Create packages with various test structures
	packages := []struct {
		name      string
		tests     []string
		subtests  map[string][]string
	}{
		{
			name:  "github.com/example/pkg1",
			tests: []string{"TestA", "TestB"},
			subtests: map[string][]string{
				"TestA": {"TestA/sub1", "TestA/sub2", "TestA/sub3"},
				"TestB": {"TestB/sub1", "TestB/sub2"},
			},
		},
		{
			name:  "github.com/example/pkg2",
			tests: []string{"TestMain"},
			subtests: map[string][]string{
				"TestMain": {"TestMain/case1", "TestMain/case2", "TestMain/case3"},
			},
		},
	}

	totalTestsRun := 0

	for _, pkg := range packages {
		model.processEvent(&types.TestEvent{
			Action:  "start",
			Package: pkg.name,
		})

		for _, test := range pkg.tests {
			// Run parent test
			model.processEvent(&types.TestEvent{
				Action:  "run",
				Package: pkg.name,
				Test:    test,
			})
			totalTestsRun++

			// Run subtests
			for _, subtest := range pkg.subtests[test] {
				model.processEvent(&types.TestEvent{
					Action:  "run",
					Package: pkg.name,
					Test:    subtest,
				})
				totalTestsRun++

				model.processEvent(&types.TestEvent{
					Action:  "pass",
					Package: pkg.name,
					Test:    subtest,
					Elapsed: 0.01,
				})
			}

			model.processEvent(&types.TestEvent{
				Action:  "pass",
				Package: pkg.name,
				Test:    test,
				Elapsed: 0.1,
			})
		}

		model.processEvent(&types.TestEvent{
			Action:  "pass",
			Package: pkg.name,
			Elapsed: 0.2,
		})
	}

	// Count what should be displayed (all tests)
	displayableTests := 0
	for _, pkg := range packages {
		if pkgResult := model.packageResults[pkg.name]; pkgResult != nil {
			displayableTests += len(pkgResult.TestOrder)
		}
	}

	// THIS TEST SHOULD FAIL UNTIL BUG IS FIXED
	// The displayed test count should match the total tests run
	assert.Equal(t, totalTestsRun, displayableTests,
		"Number of displayable tests should match total tests run")
	
	assert.Equal(t, 11, totalTestsRun, "Should have run 11 tests total")
	assert.Equal(t, 11, model.passCount, "Should have counted 11 passed tests")
	assert.Equal(t, 11, displayableTests, 
		"Should have 11 tests in TestOrder for display (currently fails with only 3)")
}

// TestAllTestsAreDisplayed verifies that all tests (including subtests)
// appear in the display output. This test SHOULD FAIL until the bug is fixed.
func TestAllTestsAreDisplayed(t *testing.T) {
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

	pkg := "github.com/example/pkg"
	
	// Create a package with subtests
	model.processEvent(&types.TestEvent{Action: "start", Package: pkg})
	
	// Table-driven test with multiple subtests
	subtests := []string{
		"TestTable/empty_input",
		"TestTable/valid_input", 
		"TestTable/invalid_input",
		"TestTable/special_chars",
		"TestTable/max_length",
	}
	
	// Run all subtests (parent might not get a run event in table tests)
	for _, test := range subtests {
		model.processEvent(&types.TestEvent{
			Action:  "run",
			Package: pkg,
			Test:    test,
		})
	}
	
	// Complete all subtests
	for _, test := range subtests {
		model.processEvent(&types.TestEvent{
			Action:  "pass",
			Package: pkg,
			Test:    test,
			Elapsed: 0.01,
		})
	}
	
	model.processEvent(&types.TestEvent{
		Action:  "pass",
		Package: pkg,
		Elapsed: 0.05,
	})

	pkgResult := model.packageResults[pkg]
	require.NotNil(t, pkgResult)
	
	// Generate the display output
	display := model.displayPackageResult(pkgResult)
	
	// THIS TEST SHOULD FAIL UNTIL BUG IS FIXED
	// All test names should appear in the display
	for _, test := range subtests {
		// Extract just the subtest name
		parts := strings.Split(test, "/")
		subtestName := parts[len(parts)-1]
		
		assert.Contains(t, display, subtestName,
			"Display output should contain test name '%s'", subtestName)
	}
	
	// Should not show "No tests" when tests exist
	assert.NotContains(t, display, "No tests",
		"Should not show 'No tests' when 5 tests ran")
	
	// Should show the correct count
	assert.Contains(t, display, "5 tests", 
		"Should indicate 5 tests in the summary")
}

// TestPackagesWithOnlySubtestsShowTests verifies that packages containing
// only subtests still display the test names. This test SHOULD FAIL until fixed.
func TestPackagesWithOnlySubtestsShowTests(t *testing.T) {
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

	model.processEvent(&types.TestEvent{
		Action:  "start",
		Package: pkg,
	})

	// Only subtests run (common pattern in table-driven tests)
	subtests := []string{
		"TestIntegration/connect_to_database",
		"TestIntegration/create_user",
		"TestIntegration/update_user",
		"TestIntegration/delete_user",
		"TestIntegration/cleanup",
	}

	for _, test := range subtests {
		model.processEvent(&types.TestEvent{
			Action:  "run",
			Package: pkg,
			Test:    test,
		})
	}

	for _, test := range subtests {
		model.processEvent(&types.TestEvent{
			Action:  "pass",
			Package: pkg,
			Test:    test,
			Elapsed: 0.5,
		})
	}

	model.processEvent(&types.TestEvent{
		Action:  "pass",
		Package: pkg,
		Elapsed: 2.5,
	})

	pkgResult := model.packageResults[pkg]
	require.NotNil(t, pkgResult)

	// THIS TEST SHOULD FAIL UNTIL BUG IS FIXED
	// TestOrder should contain all subtests even without parent
	// Note: The parent test might be implicitly added when processing subtests
	t.Logf("TestOrder contents: %v", pkgResult.TestOrder)
	assert.GreaterOrEqual(t, len(pkgResult.TestOrder), 5,
		"TestOrder should contain at least the 5 subtests")

	// Generate display
	display := model.displayPackageResult(pkgResult)

	// All subtest names should be visible
	assert.Contains(t, display, "connect_to_database", 
		"Should display 'connect_to_database' subtest")
	assert.Contains(t, display, "create_user",
		"Should display 'create_user' subtest")
	assert.Contains(t, display, "update_user",
		"Should display 'update_user' subtest")
	
	// Should NOT show "No tests" or be blank
	assert.NotContains(t, display, "No tests",
		"Should not show 'No tests' when subtests exist")
	
	// Should show test details, not just summary
	assert.NotEqual(t, strings.TrimSpace(display), pkg,
		"Display should show more than just the package name")
}

// TestTotalCountAccuracy verifies that the total count in the summary
// accurately reflects all tests including subtests. This currently PASSES
// but is included to ensure the counting logic remains correct.
func TestTotalCountAccuracy(t *testing.T) {
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

	// Run a mix of tests
	pkg := "github.com/example/mixed"
	model.processEvent(&types.TestEvent{Action: "start", Package: pkg})
	
	// Regular test
	model.processEvent(&types.TestEvent{Action: "run", Package: pkg, Test: "TestRegular"})
	model.processEvent(&types.TestEvent{Action: "pass", Package: pkg, Test: "TestRegular", Elapsed: 0.1})
	
	// Test with subtests
	model.processEvent(&types.TestEvent{Action: "run", Package: pkg, Test: "TestWithSubs"})
	model.processEvent(&types.TestEvent{Action: "run", Package: pkg, Test: "TestWithSubs/sub1"})
	model.processEvent(&types.TestEvent{Action: "run", Package: pkg, Test: "TestWithSubs/sub2"})
	model.processEvent(&types.TestEvent{Action: "pass", Package: pkg, Test: "TestWithSubs/sub1", Elapsed: 0.1})
	model.processEvent(&types.TestEvent{Action: "pass", Package: pkg, Test: "TestWithSubs/sub2", Elapsed: 0.1})
	model.processEvent(&types.TestEvent{Action: "pass", Package: pkg, Test: "TestWithSubs", Elapsed: 0.2})
	
	// Skipped test
	model.processEvent(&types.TestEvent{Action: "run", Package: pkg, Test: "TestSkipped"})
	model.processEvent(&types.TestEvent{Action: "skip", Package: pkg, Test: "TestSkipped", Elapsed: 0})
	
	model.processEvent(&types.TestEvent{Action: "pass", Package: pkg, Elapsed: 0.3})
	
	// Verify counts
	assert.Equal(t, 4, model.passCount, "Should count 4 passed tests")
	assert.Equal(t, 1, model.skipCount, "Should count 1 skipped test")
	assert.Equal(t, 5, model.passCount+model.skipCount, "Total should be 5")
	
	// Generate summary
	summary := model.GenerateFinalSummary()
	assert.Contains(t, summary, "Total:         5", "Summary should show total of 5 tests")
}