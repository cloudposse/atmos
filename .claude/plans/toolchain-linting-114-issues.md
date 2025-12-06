# Toolchain Linting Remediation Plan (114 Issues)

## Overview

**Scope**: Fix 114 linting issues in toolchain-only files
**Files affected**: toolchain/, cmd/toolchain/ directories only
**Strategy**: Fix from simplest to most complex, commit frequently

---

## Issue Breakdown

| Category | Count | Difficulty | Priority |
|----------|-------|------------|----------|
| godot | 8 | Easy | High (quick win) |
| errcheck | 10 | Easy | High (quick win) |
| errorlint | 3 | Easy | High (quick win) |
| err113 | 2 | Easy | High (quick win) |
| nilerr | 1 | Easy | High (quick win) |
| gosec | 1 | Easy | High (quick win) |
| dupl | 4 | Medium | Medium (refactor) |
| gocritic | 14 | Medium | Medium (refactor) |
| nestif | 10 | Medium | Low (style) |
| gocognit | 12 | Hard | Low (complexity) |
| revive | 49 | Mixed | Low (style) |

**Total**: 114 issues

---

## Phase 1: Quick Wins (25 issues)

**Goal**: Fix straightforward issues that don't require refactoring

### 1.1 godot - Comment Formatting (8 issues)

**Files**:
- cmd/toolchain/toolchain_test.go:610
- toolchain/installer.go:203
- toolchain/registry/aqua/aqua.go:576
- toolchain/set.go:23,26
- And 3 more

**Fix**: Add periods to comments, capitalize first letter

**Commit**: `fix(toolchain): Add periods and capitalization to comments (godot)`

---

### 1.2 errcheck - Ignored Returns (10 issues)

**Files**:
- cmd/toolchain/registry/list.go:257 - `ui.Warningf`
- cmd/toolchain/registry/search.go:198,324 - `ui.Warningf`, `ui.Writeln`
- toolchain/info.go:120 - `data.Write`
- toolchain/progress.go:18,26,35,37 - `ui.Write`, `ui.Writeln`
- toolchain/uninstall.go:321,330 - `ui.Writeln`

**Fix**: Add `_ = ` before UI calls (intentionally ignored)

**Commit**: `fix(toolchain): Acknowledge intentionally ignored UI returns (errcheck)`

---

### 1.3 errorlint - Error Wrapping (3 issues)

**Files**:
- toolchain/env.go:30
- toolchain/path.go:30
- toolchain/uninstall.go:247

**Fix**: Change `%v` to `%w` in fmt.Errorf

**Commit**: `fix(toolchain): Use %w for proper error wrapping (errorlint)`

---

### 1.4 err113 - Dynamic Errors (2 issues)

**Files**:
- toolchain/registry/aqua/aqua.go:971
- toolchain/registry/url.go:385

**Issue**: `fmt.Errorf("unsupported version constraint format: %q", constraint)`

**Fix**: Define static error in errors/errors.go, wrap with context

```go
// errors/errors.go
var ErrUnsupportedVersionConstraint = errors.New("unsupported version constraint format")

// Usage
return fmt.Errorf("%w: %q", errUtils.ErrUnsupportedVersionConstraint, constraint)
```

**Commit**: `fix(toolchain): Use static error for version constraint (err113)`

---

### 1.5 nilerr - Nil Error Check (1 issue)

**File**: TBD (need to check specific line)

**Fix**: Return error instead of nil when error variable is set

**Commit**: `fix(toolchain): Return error instead of nil (nilerr)`

---

### 1.6 gosec - Security Issue (1 issue)

**File**: TBD (need to check specific line)

**Fix**: Address security concern (likely file permissions or unsafe operation)

**Commit**: `fix(toolchain): Address security issue (gosec)`

---

## Phase 2: Medium Complexity (28 issues)

**Goal**: Refactor code for better practices without major restructuring

### 2.1 dupl - Duplicate Code (4 issues)

**Duplicates**:
1. cmd/toolchain/registry/list.go:400-440 ↔ search.go:328-368
2. cmd/toolchain/registry/provider_test.go:50-80 ↔ 83-113

**Fix**: Extract common logic into shared functions

**Commit**: `refactor(toolchain): Extract duplicate code into shared functions (dupl)`

---

### 2.2 gocritic - Code Quality (14 issues)

**Categories**:
- **ifElseChain** (4): Rewrite if-else to switch
  - cmd/toolchain/registry/list.go:308
  - cmd/toolchain/registry/search.go:250
  - toolchain/info.go:235
  - toolchain/installer.go:482
  - toolchain/registry/aqua/aqua.go:869

- **hugeParam** (6): Pass large structs by pointer
  - toolchain/get.go:125 (552 bytes)
  - toolchain/install.go:324 (784 bytes)
  - toolchain/list.go:383 (7344 bytes)
  - toolchain/set.go:72,202 (21752 bytes)
  - toolchain/set.go:500 (3912 bytes)

- **rangeValCopy** (2): Use pointers in range loops
  - toolchain/registry/url.go:236 (256 bytes)
  - toolchain/registry/url.go:305 (160 bytes)

- **nestingReduce** (1): Invert if + continue
  - toolchain/set_test.go:467

- **deep-exit** (1): Move os.Exit to main/init
  - toolchain/exec.go:26

**Commits**:
```bash
git commit -m "refactor(toolchain): Convert if-else chains to switch statements (gocritic)"
git commit -m "refactor(toolchain): Pass large structs by pointer (gocritic)"
git commit -m "refactor(toolchain): Use pointers in range loops (gocritic)"
git commit -m "refactor(toolchain): Reduce nesting with early returns (gocritic)"
git commit -m "fix(toolchain): Move os.Exit to main function (gocritic)"
```

---

### 2.3 nestif - Nested Conditionals (10 issues)

**Strategy**: Reduce nesting depth with early returns

**Commit**: `refactor(toolchain): Reduce nested conditionals with early returns (nestif)`

---

## Phase 3: Complex Refactoring (61 issues)

**Goal**: Address complexity and extensive style issues

### 3.1 gocognit - Cognitive Complexity (12 issues)

**High complexity functions**:
- toolchain/list.go:39 - RunList (65)
- toolchain/set.go:78 - Update (43)
- cmd/toolchain/registry/list.go:242 - buildToolsTable (43)
- cmd/toolchain/registry/search.go:189 - displaySearchResults (40)
- toolchain/info.go:30 - InfoExec (35)
- toolchain/registry/aqua/aqua.go:816 - parseIndexYAML (29)
- toolchain/tool_versions.go:182 - wouldCreateDuplicate (29)
- toolchain/install.go:163 - InstallSingleTool (28)
- toolchain/set.go:255 - SetToolVersion (21)
- toolchain/env.go:19 - EmitEnv (21)
- toolchain/path.go:19 - EmitPath (21)

**Strategy**:
1. Extract helper functions for logical sub-tasks
2. Simplify conditional logic
3. Use early returns to reduce nesting
4. Break down into smaller, focused functions

**Commits** (one per major function):
```bash
git commit -m "refactor(toolchain): Reduce RunList complexity (gocognit)"
git commit -m "refactor(toolchain): Reduce Update complexity (gocognit)"
git commit -m "refactor(toolchain): Reduce buildToolsTable complexity (gocognit)"
# ... etc
```

---

### 3.2 revive - Style Issues (49 issues)

**Categories**:
- **add-constant** (~15): Extract magic numbers/strings
- **cyclomatic** (~5): Reduce cyclomatic complexity
- **deep-exit** (1): Move os.Exit
- **Other style** (~28): Various formatting/naming

**Strategy**:
1. Extract magic numbers to named constants
2. Simplify complex functions
3. Fix naming/formatting issues

**Commits** (grouped by subcategory):
```bash
git commit -m "refactor(toolchain): Extract magic numbers to constants (revive)"
git commit -m "refactor(toolchain): Reduce cyclomatic complexity (revive)"
git commit -m "refactor(toolchain): Fix style issues (revive)"
```

---

## Execution Plan

### Order of Operations

1. **Phase 1: Quick Wins** (25 issues, ~6 commits)
   - godot (8)
   - errcheck (10)
   - errorlint (3)
   - err113 (2)
   - nilerr (1)
   - gosec (1)

2. **Phase 2: Medium** (28 issues, ~6-8 commits)
   - dupl (4)
   - gocritic (14)
   - nestif (10)

3. **Phase 3: Complex** (61 issues, ~15-20 commits)
   - gocognit (12)
   - revive (49)

**Total estimated commits**: 27-34 commits

---

## Verification Steps (After Each Commit)

```bash
# Verify only toolchain files modified
git diff --name-only HEAD~1..HEAD | grep -v "^toolchain/\|^cmd/toolchain/\|^errors/"
# Should return nothing (errors/ allowed for static error definitions)

# Run linter on toolchain files only
./custom-gcl run 2>&1 | grep -E "^(toolchain/|cmd/toolchain/)" | wc -l
# Should decrease after each commit

# Verify build still works
make build

# Verify tests pass
go test ./toolchain/... ./cmd/toolchain/...
```

---

## Commit Strategy

**Format**: `<type>(toolchain): <description> (<linter>)`

**Examples**:
- `fix(toolchain): Add periods to comments (godot)`
- `refactor(toolchain): Extract duplicate search logic (dupl)`
- `refactor(toolchain): Reduce RunList complexity (gocognit)`

**Rules**:
1. One commit per logical group of fixes
2. Keep commits focused (single linter category when possible)
3. Test after each commit
4. Push after each phase completion

---

## Success Criteria

✅ All 114 toolchain linting issues resolved
✅ Only toolchain/ and cmd/toolchain/ files modified (+ errors/ for static errors)
✅ Build succeeds: `make build`
✅ Tests pass: `go test ./toolchain/... ./cmd/toolchain/...`
✅ Linter clean for toolchain: `./custom-gcl run 2>&1 | grep -E "^(toolchain/|cmd/toolchain/)" | wc -l` → 0
✅ Pre-commit hooks pass
✅ Git history clean with focused commits
