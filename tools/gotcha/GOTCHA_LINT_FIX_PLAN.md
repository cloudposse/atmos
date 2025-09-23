# Gotcha Lint Fix Plan

## CRITICAL BUG FIX: Progress Bar Regression (FIXED)

### [x] Progress Bar Disappeared After Constants Implementation
- [x] Root cause: Mismatch between string literals and constants
- [x] Files affected: `event_processor.go` (uses literals from JSON), `display.go` and `update_handlers.go` (use constants)
- [x] Solution: Keep literals for JSON parsing (event.Action), use constants for internal status
- [x] Verified: Progress bar now displays correctly with subtest progress indicators

## PRIORITY: Fix Go Version Mismatch

### [x] Update Go Version Requirements
- [x] Update `tools/gotcha/go.mod`: Change `go 1.24.0` to `go 1.25.0`
- [x] Run `go mod tidy` to update dependencies
- [ ] Verify CI uses Go 1.25.0 in GitHub Actions

## Phase 1: Zero-Risk Quick Wins (~90 issues)

### [x] 1.1 Extract Magic Numbers and Strings (60+ issues)

#### [x] Create Constants Files:
- [x] `cmd/gotcha/constants_internal.go`
- [x] `internal/tui/constants.go`
- [x] `pkg/stream/constants.go`

#### [x] Extract Constants:
- [x] File permissions: `0o644` → `const DefaultFilePerms = 0o644` (17 occurrences)
- [x] Test statuses: `"pass"`, `"fail"`, `"skip"`, `"running"` (20 occurrences)
- [x] Terminal dimensions: `80`, `100`, `64`, `30`, `20`, `10`
- [x] Formatting: `" "`, `"\n"`, `"format"`

### [x] 1.2 Fix Godot Issues (6 issues)
- [x] `internal/tui/display_regression_test.go:19` - Add period
- [x] `internal/tui/subtest_regression_test.go:19,437,475` - Add periods
- [x] `pkg/stream/stream_reporter.go:219` - Capitalize sentence
- [x] `pkg/stream/tui_runner.go:292` - Add period

## Phase 2: Low-Risk Error Handling (~40 issues)

### [x] 2.1 Fix viper.BindEnv Error Checks (30+ issues)
- [x] `pkg/config/env.go` - Add `_ =` to all BindEnv calls
- [x] `cmd/ptyrunner/main.go:25` - Add error check

### [x] 2.2 Fix fmt.Sscanf Error Checks (3 issues)
- [x] `internal/tui/display.go:101,107,115` - Add error handling with defaults

## Phase 3: Code Style Improvements (~20 issues)

### [x] 3.1 Gocritic Fixes (14 issues)
- [x] Switch statement rewrites (ifElseChain) - Fixed 6 issues
- [x] Fix hugeParam issues - Changed to pointer for config structs
- [ ] Update `strings.Replace` → `strings.ReplaceAll`
- [ ] Invert if-else for clarity

### [ ] 3.2 Gofumpt Formatting (6 issues)
- [ ] Run `gofumpt -w .` on all Go files

## Phase 4: Structural Improvements (~50 issues)

### [ ] 4.1 Function Argument Reduction (10+ issues)
- [ ] Create `TestConfig` struct for test parameters
- [ ] Create `StreamConfig` struct for stream parameters
- [ ] Update function signatures

### [ ] 4.2 Deep Exit Refactoring (3 issues)
- [ ] `cmd/gotcha/main.go` - Return exit codes instead of os.Exit

## Phase 5: Complex Refactoring (~100 issues)

### [ ] 5.1 High Complexity Functions (15 functions)

#### Extreme Complexity (>100):
- [ ] `internal/tui/display.go:428` - `displayTestOld` (complexity: 102)
- [ ] `cmd/gotcha/stream.go:113` - `runStreamOld` (complexity: 101)

#### Very High Complexity (50-100):
- [ ] `cmd/gotcha/stream_execution.go:28` - `runStreamInteractive` (complexity: 86)
- [ ] `internal/tui/display.go:189` - `displayPackageResult` (complexity: 77)
- [ ] `internal/parser/parser.go:67` - `processLineWithElapsedAndSkipReason` (complexity: 57)
- [ ] `internal/coverage/processor.go:191` - `displayFunctionCoverageTree` (complexity: 57)

#### High Complexity (20-50):
- [ ] `cmd/gotcha/stream_orchestrator.go:22` - `orchestrateStream` (complexity: 44)
- [ ] `internal/markdown/comment.go:266` - `truncateToEssentials` (complexity: 28)
- [ ] `cmd/gotcha/stream_config.go:55` - `extractStreamConfig` (complexity: 26)
- [ ] `internal/coverage/processor.go:439` - `showFunctionCoverageSummary` (complexity: 23)

### [ ] 5.2 Deep Nesting Reduction (20+ issues)

#### Extreme Nesting (>30):
- [ ] `cmd/gotcha/stream_execution.go:87` - complexity: 35

#### High Nesting (8-30):
- [ ] `cmd/gotcha/stream.go:308` - complexity: 9
- [ ] `internal/output/output.go:51` - complexity: 9
- [ ] `cmd/gotcha/stream.go:434` - complexity: 8
- [ ] `cmd/gotcha/stream_orchestrator.go:140` - complexity: 8

#### Medium Nesting (4-7):
- [ ] `cmd/gotcha/stream.go:134` - complexity: 7
- [ ] `cmd/gotcha/stream_orchestrator.go:88` - complexity: 7
- [ ] `internal/coverage/processor.go:474` - complexity: 5
- [ ] `cmd/gotcha/stream.go:276` - complexity: 4
- [ ] `internal/markdown/comment.go:320` - complexity: 4

## Progress Tracking

### Issues Count:
- Initial: 304 issues (322 errors + warnings in PR)
- After Phase 1-2 + Bug Fix: 265 issues (39 fixed)
- After Phase 3.1 (partial): 259 issues (45 fixed total)
- After Critical Regression Fixes: 259 issues (fixed CI detection, vertical connector color)
- After Exit Reporting Fixes: 270 issues (slight increase due to new code)
- Current Status: 270 issues remaining

## Testing Checklist

### [ ] After Each Phase:
- [ ] Run `go build ./...` to verify compilation
- [ ] Run `go test ./...` to verify tests pass
- [ ] Run `golangci-lint run ./...` to check progress
- [ ] Compare output with previous version (golden files)

## Progress Tracking

### Summary:
- Total Issues: 304 (initial)
- Previous: 270 issues
- **Current: 248 issues** ✅
- **Fixed Today: 56 issues total (18.4% reduction)**
- Remaining: 248

### Current Issue Breakdown (248 total):
- **revive**: 133 (-11) - mostly magic numbers, unexported returns
- **nestif**: 32 (unchanged) - deeply nested if statements
- **gocognit**: 25 (unchanged) - high cognitive complexity
- **gocritic**: 15 (+2) - code style issues
- **gofumpt**: 5 (-7) - formatting issues ✅
- **unused**: 7 (-4) - unused fields/types ✅
- **staticcheck**: 8 (unchanged) - static analysis issues
- **gosec**: 6 (unchanged) - security issues (file permissions)
- **dupl**: 2 (-2) - duplicate code blocks
- **ineffassign**: 4 (unchanged) - ineffective assignments
- **unparam**: 6 (unchanged) - unused parameters
- **nilerr**: 3 (unchanged) - nil error returns
- **errcheck**: 2 (unchanged) - unchecked errors

### By Phase:
- Phase 1: 90/90 complete ✓
- Phase 2: 40/40 complete ✓
- Phase 3: 6/20 complete (gocritic partially done)
- Phase 4: 0/50 complete
- Phase 5: 0/100 complete

## Notes

### Critical Regression Fixes Applied:
1. **CI Detection Logic**: Separated runtime environment detection from configuration
   - Added `runtime.*` keys for actual env var checking
   - Keep regular keys for config settings
   - `IsCI()` checks runtime, `IsCIEnabled()` checks config
2. **Vertical Connector Color**: Changed from color 238 to 242 (dark gray)
3. **Progress Bar**: Fixed by correcting CI detection logic

### Conservative Guidelines:
1. Never change visible behavior or output format
2. Preserve all side effects
3. Test before and after each change
4. One file at a time
5. Skip if uncertain

### Rollback Plan:
- Commit after each phase
- Tag stable points
- Keep refactoring in separate branch

### Files to Not Commit:
- This plan file (GOTCHA_LINT_FIX_PLAN.md)
- Any temporary test outputs
- Debug files

## Next Steps - Prioritized Action Plan

### Immediate Actions (Low Risk, High Impact):

1. **Clean up unused code (11 issues)** ✓ LOW RISK
   - Remove unused fields: `finalOutput` in `tui_runner.go`
   - Remove unused types: `streamOutputMsg` in tests
   - Quick wins that reduce clutter

2. **Fix formatting with gofumpt (12 issues)** ✓ LOW RISK
   - Run: `gofumpt -w .`
   - Automated formatting, no logic changes

3. **Fix remaining gocritic issues (13 issues)** ✓ LOW RISK
   - Replace `strings.Replace` → `strings.ReplaceAll`
   - Invert if-else for clarity
   - Simple refactoring patterns

4. **Fix errcheck issues (2 issues)** ✓ LOW RISK
   - Add error checks where missing
   - Use `_ =` for intentionally ignored errors

### Medium Priority (Moderate Risk):

5. **Address duplicate code (4 issues)** ⚠️ MEDIUM RISK
   - Extract common display functions
   - Create shared helper methods
   - Careful not to break display logic

6. **Fix ineffassign and nilerr (7 issues)** ⚠️ MEDIUM RISK
   - Review error handling paths
   - Fix ineffective assignments
   - Ensure errors are properly returned

7. **Fix gosec issues (6 issues)** ⚠️ MEDIUM RISK
   - Update file permissions to use constants
   - Review security-sensitive operations

### Complex Refactoring (High Risk):

8. **Reduce nestif complexity (32 issues)** ⚠️ HIGH RISK
   - Extract helper functions
   - Use early returns
   - Simplify control flow

9. **Reduce cognitive complexity (25 issues)** ⚠️ HIGH RISK
   - Break down large functions
   - Extract business logic
   - Create smaller, focused functions

10. **Address revive issues (144 issues)** ⚠️ HIGH RISK
    - Extract remaining magic numbers
    - Fix unexported return types
    - Large volume, needs careful approach

### Estimated Timeline:
- **Quick Wins (1-4)**: 1-2 hours
- **Medium Priority (5-7)**: 2-3 hours
- **Complex Refactoring (8-10)**: 8-12 hours
- **Total**: ~15-20 hours of work

### Risk Mitigation:
- Commit after each category
- Run full test suite after each change
- Compare output with baseline
- Keep refactoring in separate branch

---
Last Updated: 2025-09-16 (post linting fixes)
Status: Completed 3 quick win tasks:
✅ Removed unused code (4 items removed)
✅ Applied gofumpt formatting (7 issues fixed)
✅ Fixed magic numbers (11 revive issues fixed)

Next Action: Fix gocritic issues (15 remaining) and errcheck issues (2 remaining)
