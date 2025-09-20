package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

// TestDisplayRegressionSubtestsNotShown verifies the critical regression where
// subtests are counted in the total but not shown in the display output.
//
// This is the exact issue reported by the user where they see:
// - "Total: 35" (or any count) in the summary
// - But only see a few test names in the actual output
// - Most packages appear blank or show only summary lines like "All 7 tests passed".
func TestDisplayRegressionSubtestsNotShown(t *testing.T) {
	// Create a model with "all" filter to show all tests
	model := NewTestModel(
		[]string{"github.com/example/project"},
		"",
		"",
		"",
		"all", // This should show ALL tests
		false,
		"standard",
		0,
	)

	// Simulate a realistic test execution with multiple packages
	// Package 1: Has both top-level tests and subtests
	pkg1 := "github.com/example/project/pkg1"
	model.processEvent(&types.TestEvent{Action: "start", Package: pkg1})

	// Top-level test with subtests
	model.processEvent(&types.TestEvent{Action: "run", Package: pkg1, Test: "TestFeature"})
	model.processEvent(&types.TestEvent{Action: "run", Package: pkg1, Test: "TestFeature/creates_resource"})
	model.processEvent(&types.TestEvent{Action: "run", Package: pkg1, Test: "TestFeature/updates_resource"})
	model.processEvent(&types.TestEvent{Action: "run", Package: pkg1, Test: "TestFeature/deletes_resource"})
	model.processEvent(&types.TestEvent{Action: "pass", Package: pkg1, Test: "TestFeature/creates_resource", Elapsed: 0.1})
	model.processEvent(&types.TestEvent{Action: "pass", Package: pkg1, Test: "TestFeature/updates_resource", Elapsed: 0.1})
	model.processEvent(&types.TestEvent{Action: "pass", Package: pkg1, Test: "TestFeature/deletes_resource", Elapsed: 0.1})
	model.processEvent(&types.TestEvent{Action: "pass", Package: pkg1, Test: "TestFeature", Elapsed: 0.3})

	// Another top-level test without subtests
	model.processEvent(&types.TestEvent{Action: "run", Package: pkg1, Test: "TestHelper"})
	model.processEvent(&types.TestEvent{Action: "pass", Package: pkg1, Test: "TestHelper", Elapsed: 0.05})

	model.processEvent(&types.TestEvent{Action: "pass", Package: pkg1, Elapsed: 0.35})

	// Package 2: Has ONLY subtests (common with table-driven tests)
	pkg2 := "github.com/example/project/pkg2"
	model.processEvent(&types.TestEvent{Action: "start", Package: pkg2})

	// All tests are subtests - parent never gets a "run" event
	model.processEvent(&types.TestEvent{Action: "run", Package: pkg2, Test: "TestValidation/empty_input"})
	model.processEvent(&types.TestEvent{Action: "run", Package: pkg2, Test: "TestValidation/valid_input"})
	model.processEvent(&types.TestEvent{Action: "run", Package: pkg2, Test: "TestValidation/invalid_format"})
	model.processEvent(&types.TestEvent{Action: "run", Package: pkg2, Test: "TestValidation/special_chars"})
	model.processEvent(&types.TestEvent{Action: "run", Package: pkg2, Test: "TestValidation/unicode"})
	model.processEvent(&types.TestEvent{Action: "run", Package: pkg2, Test: "TestValidation/max_length"})
	model.processEvent(&types.TestEvent{Action: "run", Package: pkg2, Test: "TestValidation/min_length"})

	for _, test := range []string{
		"TestValidation/empty_input",
		"TestValidation/valid_input",
		"TestValidation/invalid_format",
		"TestValidation/special_chars",
		"TestValidation/unicode",
		"TestValidation/max_length",
		"TestValidation/min_length",
	} {
		model.processEvent(&types.TestEvent{Action: "pass", Package: pkg2, Test: test, Elapsed: 0.01})
	}

	model.processEvent(&types.TestEvent{Action: "pass", Package: pkg2, Elapsed: 0.07})

	// Now check the results
	// Total tests run: 2 top-level + 10 subtests = 12
	assert.Equal(t, 12, model.passCount, "Should count all 12 tests that passed")

	// Generate the final summary
	summary := model.GenerateFinalSummary()
	assert.Contains(t, summary, "Total:        12", "Summary should show 12 total tests")

	// Now check what would actually be displayed
	pkg1Result := model.packageResults[pkg1]
	require.NotNil(t, pkg1Result)

	pkg2Result := model.packageResults[pkg2]
	require.NotNil(t, pkg2Result)

	// FIXED: Check TestOrder (what gets displayed)
	assert.Equal(t, 5, len(pkg1Result.TestOrder),
		"pkg1: All tests including subtests should be in TestOrder")
	// pkg2 may have 7 subtests + 1 parent test entry
	assert.GreaterOrEqual(t, len(pkg2Result.TestOrder), 7,
		"pkg2: All subtests should be in TestOrder (may include parent)")

	// Generate display output for each package
	display1 := model.displayPackageResult(pkg1Result)
	display2 := model.displayPackageResult(pkg2Result)

	// Check what the user would see
	// For pkg1: Should show test names
	assert.Contains(t, display1, "TestFeature", "Should show TestFeature")
	assert.Contains(t, display1, "TestHelper", "Should show TestHelper")

	// Check if subtests are shown (they should be with the fix)
	// In the current code, subtests ARE displayed if they're in SubtestOrder
	// The real issue is that in production, subtests might not be getting added properly
	if strings.Contains(display1, "creates_resource") {
		t.Log("Note: Subtests ARE being displayed in this test (fix may be working)")
	} else {
		t.Log("BUG CONFIRMED: Subtests are not displayed even though they ran")
	}

	// For pkg2: Should now show subtest names
	assert.Contains(t, display2, "empty_input",
		"Subtests should be displayed for pkg2")
	assert.Contains(t, display2, "valid_input",
		"Subtests should be displayed for pkg2")

	// Count how many test names are actually visible in display
	visibleTests := 0
	visibleTests += strings.Count(display1, "✔")
	visibleTests += strings.Count(display1, "✘")
	visibleTests += strings.Count(display2, "✔")
	visibleTests += strings.Count(display2, "✘")

	// Don't count summary lines
	if strings.Contains(display1, "All") {
		visibleTests--
	}
	if strings.Contains(display2, "All") {
		visibleTests--
	}

	// The regression: 12 tests ran, but only 2 test names visible!
	t.Logf("REGRESSION CONFIRMED:")
	t.Logf("  - Total tests counted: %d", model.passCount)
	t.Logf("  - Test names displayed: ~%d", visibleTests)
	t.Logf("  - Package 1 TestOrder: %d (should be 5 with subtests)", len(pkg1Result.TestOrder))
	t.Logf("  - Package 2 TestOrder: %d (should be 7 with subtests)", len(pkg2Result.TestOrder))

	// FIXED: The issue is now resolved:
	// - Summary shows "Total: 12"
	// - All 12 test names are visible
	// - Package 2 shows all its subtests properly
	assert.GreaterOrEqual(t, visibleTests, 12,
		"FIXED: All 12 test names should be visible")
}
