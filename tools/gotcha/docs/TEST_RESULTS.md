# Gotcha Subtest Display Bug - Test Results

## Summary

We've successfully created regression tests that reproduce the subtest display bug reported by the user. These tests are properly written to **FAIL** with the current broken behavior and will **PASS** once the bug is fixed.

## Test Results

All tests correctly fail, confirming the bug:

```
--- FAIL: TestSubtestsAreInTestOrder (0.00s)
--- FAIL: TestCountMatchesDisplay (0.00s)
--- FAIL: TestAllTestsAreDisplayed (0.00s)
--- FAIL: TestPackagesWithOnlySubtestsShowTests (0.00s)
--- PASS: TestTotalCountAccuracy (0.00s)
```

## What Each Test Verifies

### 1. `TestSubtestsAreInTestOrder` ❌ FAILS
**Expected:** Subtests should be included in `TestOrder` for display
**Actual:** Only parent test is in `TestOrder` (1 item instead of 4)
**Evidence:** `TestOrder` contains `["TestParent"]` but should contain all 4 tests including subtests

### 2. `TestCountMatchesDisplay` ❌ FAILS
**Expected:** Number of displayable tests should match total tests run (11)
**Actual:** Only 3 tests are in `TestOrder` (only top-level tests)
**Evidence:** 11 tests run, but only 3 would be displayed

### 3. `TestAllTestsAreDisplayed` ❌ FAILS
**Expected:** All test names should appear in display output
**Actual:** Subtests are not shown in the display
**Evidence:** Display output missing subtest names like "empty_input", "valid_input", etc.

### 4. `TestPackagesWithOnlySubtestsShowTests` ❌ FAILS
**Expected:** Packages with only subtests should still show test names
**Actual:** Package shows as blank or "No tests"
**Evidence:** `TestOrder` is empty (0 items) when it should have 5 subtests

### 5. `TestTotalCountAccuracy` ✅ PASSES
**Purpose:** Ensures the counting logic is correct (this part works)
**Result:** The total count correctly includes all subtests
**Note:** This is why users see "Total: 1764" but only a few test names

## The Core Problem

The bug is in `processTestRun` in `event_processor.go`:

```go
if isSubtest {
    // Subtests are added to parent's SubtestOrder, NOT to pkg.TestOrder
    parent.SubtestOrder = append(parent.SubtestOrder, event.Test)
} else {
    // Only top-level tests are added to pkg.TestOrder
    pkg.TestOrder = append(pkg.TestOrder, event.Test)
}
```

This causes:
- ✅ Subtests are counted in totals (`m.actualTestCount++`)
- ❌ Subtests are NOT in `TestOrder` for display
- Result: Count shows 1700+ but display shows ~35 test names

## User Experience

What the user sees:
```
Test Results:
  ✔ Passed:     1742
  ✘ Failed:      18
  ⊘ Skipped:     4
  Total:        1764     <-- Correct count including subtests

But in the output:
▶ github.com/cloudposse/atmos/pkg/config
  [blank - no test names shown]

▶ github.com/cloudposse/atmos/internal/exec
  ✔ All 5 tests passed    <-- Just a summary, no test names
```

## How to Run These Tests

```bash
# Run all subtest display tests
go test -v ./internal/tui -run "^TestSubtests|^TestCount|^TestAllTests|^TestPackages|^TestTotal"

# Run individual test
go test -v ./internal/tui -run TestSubtestsAreInTestOrder
```

## Success Criteria

When the bug is fixed, all tests should pass:
- Subtests will be in `TestOrder`
- Display count will match actual test count
- All test names will be visible in output
- Packages with only subtests will show test details

## Files Created

- `tools/gotcha/internal/tui/subtest_display_test.go` - Main regression tests
- `tools/gotcha/test/tui_subtest_integration_test.go` - Integration tests
- `tools/gotcha/docs/TEST_RESULTS.md` - This documentation
