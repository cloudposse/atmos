# Test Failure Analysis: Error Handling Infrastructure

## Executive Summary

The error handling infrastructure extraction introduced **7 error message regressions** where messages became less specific and actionable. Of these:
- **3 are critical** (require immediate fix)
- **2 have test failures** (detected by tests)
- **5 have no test failures** (not covered by tests - concerning!)

Additionally, there are expected **snapshot test failures** due to formatting changes (trailing whitespace, verbose flag).

---

## Critical Regressions (MUST FIX)

### 1. Terraform Flag Conflicts (CRITICAL - No Tests!)

**Impact**: Users get generic "incompatible flags" with no guidance

**Before**:
```go
ErrInvalidTerraformFlagsWithAffectedFlag = errors.New(
    "the --affected flag can't be used with the other multi-component (bulk operations) flags --all, --query and --components"
)
```

**After**:
```go
ErrInvalidTerraformFlagsWithAffectedFlag = errors.New("incompatible flags")
```

**Why This is Bad**:
- Users have NO idea which flags are incompatible
- Original message told users exactly what they can't combine
- This is a **major UX regression**

**Fix**: Revert to original detailed messages

**Files**:
- `errors/errors.go:67-69`
- Affects: `ErrInvalidTerraformFlagsWithAffectedFlag`, `ErrInvalidTerraformFlagsWithComponentArg`, `ErrInvalidTerraformFlagsForSingleComponent`

---

### 2. Git Availability Error (HAS TEST FAILURES)

**Impact**: Lost troubleshooting guidance about PATH

**Before**:
```go
ErrGitNotAvailable = errors.New("git must be available and on the PATH")
```

**After**:
```go
ErrGitNotAvailable = errors.New("git is not available")
```

**Test Failures**:
- `pkg/downloader/get_git_test.go:564`: `TestGetCustom_ErrorWhenGitMissing`
- `pkg/downloader/git_getter_test.go:277`: `TestCustomGitGetter_Get_GetCustomError`

**Expected**: `"git must be available"`
**Got**: `"git is not available"`

**Why This is Bad**:
- "on the PATH" is critical troubleshooting information
- Users need to know WHERE to make git available

**Fix**: Revert to include "and on the PATH"

**File**: `errors/errors.go:97`

---

### 3. Limit/Offset Validation (HAS TEST FAILURES)

**Impact**: Users don't know valid ranges

**Before**:
```go
ErrInvalidLimit = errors.New("limit must be between 1 and 100")
ErrInvalidOffset = errors.New("offset must be >= 0")
```

**After**:
```go
ErrInvalidLimit = errors.New("invalid limit value")
ErrInvalidOffset = errors.New("invalid offset value")
```

**Test Failures**:
- `cmd/version/list_test.go:277`: `TestListCommand_ValidationErrors/invalid_limit_too_low`
- `cmd/version/list_test.go:284`: `TestListCommand_ValidationErrors/invalid_limit_too_high`

**Expected**: `"limit must be between"`
**Got**: `"invalid limit value"`

**Why This is Bad**:
- Users need to know the valid range (1-100)
- Generic "invalid" provides no actionable guidance

**Fix**: Revert to include bounds

**File**: `errors/errors.go:51-52`

---

## Medium Priority Regressions

### 4. Stack Naming Configuration (No Tests)

**Before**:
```go
ErrMissingStackNameTemplateAndPattern = errors.New(
    "'stacks.name_pattern' or 'stacks.name_template' needs to be specified in 'atmos.yaml'"
)
```

**After**:
```go
ErrMissingStackNameTemplateAndPattern = errors.New("stack naming configuration is missing")
```

**Why This is Bad**: Users don't know which config keys to set

**Fix**: Revert to include specific keys

**File**: `errors/errors.go:46`

---

### 5. Date Format Validation (No Tests)

**Before**:
```go
ErrInvalidSinceDate = errors.New("invalid date format for --since")
```

**After**:
```go
ErrInvalidSinceDate = errors.New("invalid date format")
```

**Why This is Bad**: Lost which flag has the problem

**Fix**: Revert to include flag reference

**File**: `errors/errors.go:53`

---

## Minor Regressions

### 6. Config Path Validation (No Tests)

**Before**:
```go
ErrExpectedDirOrPattern = errors.New("--config-path expected directory found file")
ErrExpectedFile = errors.New("--config expected file found directory")
```

**After**:
```go
ErrExpectedDirOrPattern = errors.New("expected directory or pattern")
ErrExpectedFile = errors.New("expected file")
```

**Impact**: Lost CLI flag context but still reasonably clear

**Fix**: Consider reverting for consistency

**File**: `errors/errors.go:144-145`

---

## Benign Test Failures (Regenerate Snapshots)

### Snapshot Tests

**Cause**:
- Trailing whitespace removal (lipgloss formatting)
- Verbose flag additions to help text
- Terminal width differences

**Fix**:
```bash
go test ./tests -run 'TestCLICommands' -regenerate-snapshots
git diff tests/snapshots/  # Review changes
```

**Why Benign**: Output is functionally identical, just formatting differences

---

## Summary Statistics

| Category | Count | Status |
|----------|-------|--------|
| **Critical Regressions** | 3 | 🔴 Must fix |
| **Medium Regressions** | 2 | 🟡 Should fix |
| **Minor Regressions** | 2 | 🟠 Consider fixing |
| **Test Failures Detected** | 4 | ✅ Tests working |
| **Regressions Without Tests** | 5 | ⚠️ Test coverage gaps |
| **Benign Snapshot Failures** | ~10-50 | ✅ Expected |

---

## Recommended Action Plan

### Phase 1: Fix Critical Error Messages (Priority 1)

**Estimated Time**: 15 minutes

1. **Terraform flag conflicts** (`errors/errors.go:67-69`):
   ```go
   // Revert these 3 errors to original detailed messages
   ErrInvalidTerraformFlagsWithAffectedFlag = errors.New("the --affected flag can't be used with the other multi-component (bulk operations) flags --all, --query and --components")
   ErrInvalidTerraformFlagsWithComponentArg = errors.New("the component argument can't be used with the multi-component (bulk operations) flags --affected, --all, --query and --components")
   ErrInvalidTerraformFlagsForSingleComponent = errors.New("the single-component flags (--from-plan, --planfile) can't be used with the multi-component (bulk operations) flags (--affected, --all, --query, --components)")
   ```

2. **Git availability** (`errors/errors.go:97`):
   ```go
   ErrGitNotAvailable = errors.New("git must be available and on the PATH")
   ```

3. **Limit/offset validation** (`errors/errors.go:51-52`):
   ```go
   ErrInvalidLimit = errors.New("limit must be between 1 and 100")
   ErrInvalidOffset = errors.New("offset must be >= 0")
   ```

### Phase 2: Fix Medium Priority (Priority 2)

**Estimated Time**: 5 minutes

4. **Stack naming** (`errors/errors.go:46`):
   ```go
   ErrMissingStackNameTemplateAndPattern = errors.New("'stacks.name_pattern' or 'stacks.name_template' needs to be specified in 'atmos.yaml'")
   ```

5. **Date format** (`errors/errors.go:53`):
   ```go
   ErrInvalidSinceDate = errors.New("invalid date format for --since")
   ```

### Phase 3: Update Tests (Priority 3)

**Estimated Time**: 10 minutes

After fixing error messages, tests should pass. If any still fail, update them to use new messages.

**Option A** (if error messages are reverted): Tests should pass as-is

**Option B** (if keeping new messages): Update tests:
```go
// cmd/version/list_test.go:277, 284
errString: "invalid limit value",  // Was: "limit must be between"

// pkg/downloader/get_git_test.go:564, git_getter_test.go:277
require.Contains(t, err.Error(), "git is not available")  // Was: "git must be available"
```

### Phase 4: Regenerate Snapshots (Priority 4)

**Estimated Time**: 5 minutes

```bash
cd /Users/erik/Dev/cloudposse/tools/atmos/.conductor/bangui-v3
go test ./tests -run 'TestCLICommands' -regenerate-snapshots
git diff tests/snapshots/  # Review - should see verbose flag additions, whitespace fixes
git add tests/snapshots/
```

### Phase 5: Add Missing Tests (Future Work)

Create tests for the 5 untested error scenarios:
- Terraform flag conflicts (3 errors)
- Stack naming configuration
- Config path validation (2 errors)

---

## Root Cause Analysis

**Problem**: When extracting static sentinel errors from PR #1599, error message text was simplified without considering that:

1. **Sentinel errors still need descriptive messages** - The error type (`errors.Is()`) is for code, the message is for users
2. **Test coverage gaps exist** - 5 regressions went undetected because they aren't tested
3. **Context should be added via wrapping** - Not by making sentinel messages generic

**Better Pattern**:
```go
// Good: Descriptive sentinel
ErrInvalidLimit = errors.New("limit must be between 1 and 100")

// When using in code, can add context via wrapping:
return fmt.Errorf("%w: got %d", errUtils.ErrInvalidLimit, actualLimit)

// This gives both:
// - errors.Is(err, ErrInvalidLimit) works ✅
// - Users see: "limit must be between 1 and 100: got 150" ✅
```

---

## Prevention for Future PRs

1. **Review Checklist**: Add "Error message changes preserve specificity" to PR template
2. **Test Coverage**: Require tests for all error scenarios with user-facing messages
3. **Documentation**: Update error handling guide to emphasize descriptive sentinel messages
4. **Claude Agent**: Update `.claude/agents/atmos-errors.md` to flag generic messages

---

## Files to Modify

| File | Lines | Changes | Priority |
|------|-------|---------|----------|
| `errors/errors.go` | 46, 51-53, 67-69, 97 | Revert 7 error messages | P1/P2 |
| `tests/snapshots/*.golden` | Various | Regenerate | P4 |
| `cmd/version/list_test.go` | 277, 284 | Update if needed | P3 |
| `pkg/downloader/*_test.go` | 277, 564 | Update if needed | P3 |

**Total Estimated Time**: 35-45 minutes

---

## Conclusion

This is a **fixable issue** that was introduced during extraction. The fixes are straightforward:
- Revert 7 error messages to their original, more descriptive versions
- Regenerate snapshots for benign formatting changes
- All tests should then pass

The error handling infrastructure itself is sound - this is just a message wording issue that slipped through during extraction.
