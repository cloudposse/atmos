# Next Session: Fix Final 5 Lint Warnings & Boost Test Coverage to 90%

## Context

Branch: `unified-flag-parsing`
Current lint warnings: **5 remaining** (down from 165!)
Current test coverage: Unknown (needs measurement)
Target test coverage: **90%** for `pkg/flags`

## Session Goals

### Phase 1: Fix Final 5 Lint Warnings (Priority)

Fix the remaining complexity and function-length warnings by refactoring into smaller, testable functions.

#### 1. Fix cyclomatic complexity in `compatibility_translator.go` (complexity: 14 → ≤10)
```bash
# File: pkg/flags/compatibility_translator.go
# Function: (*CompatibilityAliasTranslator).translateSingleDashFlag (line 216)
# Current complexity: 14
# Target: ≤10

# Strategy:
# - Extract flag value extraction logic into helper
# - Extract alias lookup and behavior handling into helper
# - Extract separated args appending logic into helper
```

#### 2. Fix cyclomatic complexity in `flag_parser.go` (complexity: 14 → ≤10)
```bash
# File: pkg/flags/flag_parser.go
# Function: (*AtmosFlagParser).Parse (line 62)
# Current complexity: 14
# Target: ≤10

# Strategy:
# - Extract argument splitting logic (double dash handling)
# - Extract positional args extraction
# - Extract flag extraction/validation
```

#### 3. Fix cyclomatic complexity in `standard_parser.go` (complexity: 11 → ≤10)
```bash
# File: pkg/flags/standard_parser.go
# Function: (*StandardParser).Parse (line 90)
# Current complexity: 11
# Target: ≤10

# Strategy:
# - This was already partially refactored, just needs one more extraction
# - Extract arg processing logic into helper
```

#### 4. Split long function in `global_builder.go` (96 lines → ≤60)
```bash
# File: pkg/flags/global_builder.go
# Function: Line 25 (likely BuildGlobalFlags or similar)
# Current length: 96 lines
# Target: ≤60 lines

# Strategy:
# - Split into logical sections: flag registration, validation, config
# - Extract flag groups: auth flags, terminal flags, profiling flags
```

#### 5. Split long function in `global_registry.go` (141 lines → ≤60)
```bash
# File: pkg/flags/global_registry.go
# Function: Line 136 (likely GlobalFlags() or RegisterGlobalFlags)
# Current length: 141 lines
# Target: ≤60 lines

# Strategy:
# - Split by flag categories (auth, terminal, profiling, logging)
# - Create separate registration functions for each category
# - Main function orchestrates calls to helpers
```

### Phase 2: Measure Current Test Coverage

```bash
# Generate coverage report for pkg/flags
go test -short -coverprofile=coverage.out ./pkg/flags/...
go tool cover -func=coverage.out | grep total
go tool cover -html=coverage.out -o coverage.html

# View coverage by package
go test -short -cover ./pkg/flags/...

# Check coverage for specific packages
go test -short -cover ./pkg/flags/terraform/...
go test -short -cover ./pkg/flags/auth/...
```

### Phase 3: Add Tests to Reach 90% Coverage

**Focus areas for new tests:**

1. **Test all 19 new helper functions** created in previous session:
   - parseFlags(), createCombinedFlagSet(), extractArgs()
   - bindChangedFlagsToViper(), validatePositionalArgs()
   - populateFlagsFromViper(), getStringFlagValue()
   - validateSingleFlag(), isFlagExplicitlyChanged()
   - isValueValid(), createValidationError()
   - registerStringFlag(), registerIntFlag(), registerStringSliceFlag()
   - buildNoOptDefValFlagsSet(), preprocessArgs()
   - processFlagArg(), hasValueFollowing()
   - registerCustomFlag(), getFlagDescription(), registerBoolFlag(), registerIntFlag()

2. **Edge cases and error paths:**
   - Invalid flag values
   - Missing required flags
   - Conflicting flags
   - NoOptDefVal edge cases
   - Empty/nil inputs
   - Validation failures

3. **Integration scenarios:**
   - Flag inheritance (persistent flags)
   - Viper precedence (CLI > ENV > config > default)
   - Pass-through args with double dash
   - Compatibility alias translation
   - Multiple flag formats (-f, --flag, --flag=value)

**Test patterns to use:**
```go
// Table-driven tests for comprehensive coverage
func TestHelperFunction(t *testing.T) {
    tests := []struct {
        name     string
        input    X
        expected Y
        wantErr  bool
    }{
        // ... test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}

// Use testify assertions for clarity
assert.Equal(t, expected, actual)
assert.NoError(t, err)
assert.Error(t, err)
require.NotNil(t, result)
```

## Quick Reference Commands

```bash
# Check lint status
./custom-gcl run pkg/flags/... 2>&1 | grep -E "^pkg/flags"

# Count warnings
./custom-gcl run pkg/flags/... 2>&1 | grep -E "^pkg/flags" | wc -l

# Run tests
go test -short ./pkg/flags/...

# Run tests with coverage
go test -short -cover ./pkg/flags/...

# Generate coverage report
go test -short -coverprofile=coverage.out ./pkg/flags/...
go tool cover -func=coverage.out | grep total

# Build and test
go build . && go test -short ./pkg/flags/...

# Run specific package tests
go test -v ./pkg/flags/terraform/...
go test -v ./pkg/flags/standard.go -run TestStandardFlagParser
```

## Success Criteria

✅ **Zero lint warnings** in `pkg/flags` (currently 5)
✅ **90% test coverage** in `pkg/flags` (currently unknown)
✅ **All tests passing** (maintain 100% pass rate)
✅ **No breaking changes** (all existing tests still pass)
✅ **Well-documented helpers** (clear function names and comments)

## Key Files to Focus On

### Files with remaining lint warnings:
1. `pkg/flags/compatibility_translator.go`
2. `pkg/flags/flag_parser.go`
3. `pkg/flags/standard_parser.go`
4. `pkg/flags/global_builder.go`
5. `pkg/flags/global_registry.go`

### Files needing test coverage:
- All test files in `pkg/flags/` (check coverage gaps)
- Newly created helper functions (from previous refactoring)
- Edge cases in complex parsers

## Architectural Principles to Follow

1. **Single Responsibility Principle** - Each function does one thing
2. **Testability** - Small, focused functions are easier to test
3. **Options Pattern** - Use functional options for configuration
4. **Error Wrapping** - Use `fmt.Errorf("%w", err)` for error chains
5. **Performance Tracking** - Add `defer perf.Track(...)` to all functions
6. **Named Constants** - Avoid magic strings and numbers
7. **Documentation** - Clear comments on all exported functions

## Previous Session Summary

**Starting point:** 165 lint warnings
**Ending point:** 5 lint warnings
**Reduction:** 97% improvement!

**Major refactorings completed:**
- 5 complex functions broken into 19 testable helpers
- All dupl, unparam, nolintlint warnings eliminated
- Added named constants to replace magic strings/numbers
- Fixed hugeParam, QF1008, nestif warnings
- Renamed `remaining_commands.go` → `compatibility_aliases.go`

See `LINT-CLEANUP-SUMMARY.md` for complete details.

## Getting Started

```bash
# Navigate to the branch
cd /Users/erik/Dev/cloudposse/tools/atmos/.conductor/lyon
git checkout unified-flag-parsing

# Verify current state
./custom-gcl run pkg/flags/... 2>&1 | grep -E "^pkg/flags"
# Should show 5 warnings

# Start with the easiest fix
# Review the function with complexity 11 first (standard_parser.go)
# Then tackle the complexity 14 functions
# Finally split the long functions
# Then add test coverage
```

## Prompt for Claude

Use this prompt to start the next session:

---

**Prompt:**

I'm continuing work on the `unified-flag-parsing` branch. We've made outstanding progress reducing lint warnings from 165 to just 5 (97% reduction!).

Please help me:

1. **Fix the final 5 lint warnings** by refactoring complex functions:
   - 3 cyclomatic complexity warnings (need to extract helpers)
   - 2 function-length warnings (need to split into smaller functions)

2. **Measure current test coverage** in `pkg/flags`

3. **Add comprehensive test coverage** to reach 90% for `pkg/flags`

Details are in `LINT-CLEANUP-SUMMARY.md` and `NEXT-SESSION-PROMPT.md`.

Key context:
- We've already created 19 testable helper functions in the previous session
- All tests are currently passing
- Follow the architectural patterns from `CLAUDE.md`
- Use table-driven tests with testify assertions

Let's start by checking the current lint warnings and then tackle them one by one, focusing on refactoring into smaller, testable functions. After fixing the warnings, we'll measure coverage and add tests to reach 90%.

---

## Additional Resources

- **Full session summary:** `LINT-CLEANUP-SUMMARY.md`
- **Architectural patterns:** `CLAUDE.md` (in repository root)
- **Test examples:** `pkg/flags/*_test.go` files
- **Previous work:** Git history in `unified-flag-parsing` branch
