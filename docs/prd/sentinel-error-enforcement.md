# Sentinel Error Enforcement Strategy

## Executive Summary

This document outlines enforcement strategies for making sentinel errors + `errors.Is()` the mandatory, path-of-least-resistance pattern in Atmos. The goal is to make type-safe error handling easy and string-based error checking hard or impossible.

## Problem Statement

### Current State
- ✅ ErrorBuilder with `errors.Mark()` support implemented
- ✅ Auto-detection of sentinels in `Build()`
- ✅ Comprehensive documentation created
- ❌ Still ~20+ files using string-based error checking (`assert.Contains(err.Error(), ...)`)
- ❌ No automated enforcement preventing string-based error checks
- ❌ Pattern not yet ubiquitous across codebase

### Risks Without Enforcement
1. **Brittle tests**: String matching breaks when error messages change
2. **False negatives**: Tests pass even when wrong error type is returned
3. **Inconsistent patterns**: Mixed approaches make codebase harder to understand
4. **Lost type safety**: Can't check error types across wrapping layers

## Design Goals

1. **Make correct pattern obvious** - ErrorBuilder + sentinel errors should be the clear default
2. **Make incorrect pattern hard** - String-based checking should trigger warnings/errors
3. **Automated enforcement** - Linter rules catch issues at CI time, not code review
4. **Clear migration path** - Existing code can be fixed incrementally
5. **Developer education** - Linter messages should be helpful, not cryptic

## Proposed Enforcement Strategies

### Strategy 1: Custom Linter Rules (RECOMMENDED)

Add custom lintroller rules to detect and prevent string-based error checking.

**New rule: `no-error-string-matching`**

Detects and flags:
- `assert.Contains(t, err.Error(), "...")`
- `assert.Equal(t, "...", err.Error())`
- `if err.Error() == "..."` or `strings.Contains(err.Error(), ...)`
- Any other pattern that converts error to string for comparison

**Implementation:**
```go
// tools/lintroller/rule_error_string_matching.go
package lintroller

import (
    "go/ast"
    "go/token"
    "golang.org/x/tools/go/analysis"
)

// ErrorStringMatchingRule detects string-based error checking.
type ErrorStringMatchingRule struct{}

func (r *ErrorStringMatchingRule) Check(pass *analysis.Pass) {
    inspect := func(node ast.Node) bool {
        call, ok := node.(*ast.CallExpr)
        if !ok {
            return true
        }

        // Detect assert.Contains(t, err.Error(), "...")
        if isTestifyAssertContains(call) && hasErrorMethod(call) {
            pass.Reportf(call.Pos(),
                "use assert.ErrorIs(t, err, sentinel) instead of assert.Contains(err.Error(), ...)")
            return false
        }

        // Detect assert.Equal(t, "...", err.Error())
        if isTestifyAssertEqual(call) && hasErrorMethod(call) {
            pass.Reportf(call.Pos(),
                "use assert.ErrorIs(t, err, sentinel) instead of assert.Equal(..., err.Error())")
            return false
        }

        return true
    }

    for _, file := range pass.Files {
        ast.Inspect(file, inspect)
    }
}

func isTestifyAssertContains(call *ast.CallExpr) bool {
    sel, ok := call.Fun.(*ast.SelectorExpr)
    if !ok {
        return false
    }
    return sel.Sel.Name == "Contains"
}

func isTestifyAssertEqual(call *ast.CallExpr) bool {
    sel, ok := call.Fun.(*ast.SelectorExpr)
    if !ok {
        return false
    }
    return sel.Sel.Name == "Equal"
}

func hasErrorMethod(call *ast.CallExpr) bool {
    if len(call.Args) < 2 {
        return false
    }

    // Check if any argument is a call to .Error()
    for _, arg := range call.Args {
        if methodCall, ok := arg.(*ast.CallExpr); ok {
            if sel, ok := methodCall.Fun.(*ast.SelectorExpr); ok {
                if sel.Sel.Name == "Error" {
                    return true
                }
            }
        }
    }
    return false
}
```

**Configuration in `.golangci.yml`:**
```yaml
custom:
  lintroller:
    type: "module"
    description: "Atmos project-specific linting rules"
    settings:
      no-error-string-matching: true  # NEW: Enforce errors.Is() over string matching
```

**Benefits:**
- Catches issues at CI time, not code review time
- Clear, actionable error messages
- Enforces consistency across entire codebase
- Educates developers through linter output

**Drawbacks:**
- Requires custom linter implementation
- May need exceptions for third-party error types
- Initial migration effort

### Strategy 2: Pre-commit Hook (SUPPLEMENTARY)

Add pre-commit hook to detect string-based error checking locally.

**Implementation:**
```bash
# .pre-commit-config.yaml
- repo: local
  hooks:
    - id: no-error-string-matching
      name: Prevent string-based error checking
      entry: bash -c 'if grep -r "assert\.Contains.*err\.Error\|assert\.Equal.*err\.Error" --include="*.go" .; then echo "Error: Use assert.ErrorIs() instead of string-based error checking"; exit 1; fi'
      language: system
      pass_filenames: false
```

**Benefits:**
- Fast feedback loop (catches before commit)
- Simple to implement
- No additional tooling required

**Drawbacks:**
- Less sophisticated than linter (regex-based)
- Easier to bypass than CI linter
- May have false positives

### Strategy 3: Code Review Guidelines (BASELINE)

Update code review checklist to require sentinel error usage.

**Checklist additions:**
- [ ] All errors use sentinels from `errors/errors.go`
- [ ] All error checks use `errors.Is()`, not string matching
- [ ] All test assertions use `assert.ErrorIs()`, not `assert.Contains(err.Error(), ...)`
- [ ] ErrorBuilder used for all user-facing errors
- [ ] Hints and context provided for debugging

**Benefits:**
- No tooling changes required
- Human judgment applied
- Educational for reviewers and contributors

**Drawbacks:**
- Inconsistent enforcement (depends on reviewer)
- Slower feedback loop (only at PR time)
- Doesn't scale well

## Recommended Approach

**Phase 1: Documentation + Code Review (DONE)**
- ✅ Create comprehensive documentation (`docs/errors.md`)
- ✅ Update PRD (`docs/prd/error-handling-strategy.md`)
- ✅ Update `CLAUDE.md` with mandatory patterns
- ✅ Add code review checklist

**Phase 2: Pre-commit Hook (QUICK WIN)**
- [ ] Add simple grep-based pre-commit hook
- [ ] Test on existing codebase
- [ ] Document how to run locally

**Phase 3: Custom Linter (FULL ENFORCEMENT)**
- [ ] Implement `no-error-string-matching` lintroller rule
- [ ] Add comprehensive tests for linter rule
- [ ] Configure `.golangci.yml` with new rule
- [ ] Run in warning mode first

**Phase 4: Codebase Migration**
- [ ] Identify all files with string-based error checking
- [ ] Create tracking issue with file list
- [ ] Fix violations incrementally (by package)
- [ ] Track progress in weekly engineering updates

**Phase 5: Promote to Error**
- [ ] After all existing violations fixed
- [ ] Change linter severity from warning to error
- [ ] Enforce for all new code
- [ ] Document in contribution guidelines

## Migration Strategy

### Identifying Violations

```bash
# Find all files with string-based error checking
grep -r "assert\.Contains.*err\.Error" --include="*.go" . | wc -l
grep -r "assert\.Equal.*err\.Error" --include="*.go" . | wc -l
grep -r "strings\.Contains.*err\.Error" --include="*.go" . | wc -l
```

**Current count:** ~20+ files identified

### Fix Pattern

**Before (string-based):**
```go
// Test
assert.Contains(t, err.Error(), "container not found")

// Production code
if strings.Contains(err.Error(), "container") {
    // Handle container error
}
```

**After (sentinel-based):**
```go
// Test
assert.ErrorIs(t, err, errUtils.ErrContainerNotFound)

// Production code
if errors.Is(err, errUtils.ErrContainerNotFound) {
    // Handle container error
}
```

### Incremental Migration

1. **Group by package**: Fix all violations in one package at a time
2. **Update tests first**: Convert test assertions before production code
3. **Add sentinels**: Create new sentinels in `errors/errors.go` as needed
4. **Update error creation**: Use ErrorBuilder for user-facing errors
5. **Verify with CI**: Ensure tests still pass
6. **Review and merge**: One package per PR for easier review

## Linter Rule Testing

### Test Cases

```go
// Should trigger linter
assert.Contains(t, err.Error(), "not found")
assert.Equal(t, "not found", err.Error())
if err.Error() == "not found" { }
if strings.Contains(err.Error(), "not found") { }

// Should NOT trigger linter
assert.ErrorIs(t, err, ErrNotFound)
errors.Is(err, ErrNotFound)
assert.Equal(t, "expected", actualString) // Not an error type
```

## Exception Handling

### When String Matching is Acceptable

1. **Third-party errors without sentinels**:
   ```go
   // OK for external libraries without typed errors
   if strings.Contains(err.Error(), "connection refused") {
       // Handle network error from external library
   }
   ```

2. **Testing specific message formatting**:
   ```go
   // OK when explicitly testing error message format
   assert.Contains(t, err.Error(), "`vpc` not found in stack `prod`")
   ```

### Linter Exceptions

```go
// Use //nolint with justification
if strings.Contains(err.Error(), "external error") { //nolint:errcheck // third-party error without sentinel
    // ...
}
```

## Success Criteria

1. **Zero new violations**: CI blocks PRs with string-based error checking
2. **<10 legacy violations**: Existing code migrated to <10 remaining files
3. **Documentation**: All patterns documented with examples
4. **Developer feedback**: Linter messages clear and actionable
5. **Test coverage**: Linter rules have comprehensive tests

## Open Questions

### Q1: Should we add a custom linter rule immediately?

**Pros:**
- Automated enforcement from day 1
- Catches issues early in development
- Clear consistency across codebase

**Cons:**
- Implementation effort (1-2 days)
- Need to handle exceptions for third-party errors
- Linter may have false positives initially

**Recommendation:** Start with pre-commit hook (Phase 2), implement custom linter after initial migration proves the pattern works well.

### Q2: Should we do a codebase-wide sweep immediately?

**Pros:**
- Fast migration (all at once)
- Consistent pattern everywhere
- Clean slate for enforcement

**Cons:**
- Large PR (hard to review)
- Higher risk of breaking changes
- May miss edge cases

**Recommendation:** Incremental migration by package (Phase 4). Safer, easier to review, allows pattern refinement.

### Q3: Are there edge cases we need to handle?

**Potential edge cases:**
1. Third-party errors without sentinels
2. Error messages that include dynamic data
3. Errors from standard library without typed errors
4. Wrapped errors from multiple sources

**Recommendation:** Document patterns for each edge case, add linter exceptions where necessary.

## Next Steps

1. **Immediate (this PR):**
   - ✅ Review and approve documentation updates
   - ✅ Merge ErrorBuilder sentinel support
   - ✅ Update CLAUDE.md and PRDs

2. **Short-term (next sprint):**
   - [ ] Add pre-commit hook for string-based error checking
   - [ ] Create tracking issue with file list for migration
   - [ ] Start incremental migration (1-2 packages per week)

3. **Medium-term (next month):**
   - [ ] Implement custom linter rule
   - [ ] Test linter rule on migrated packages
   - [ ] Roll out linter in warning mode

4. **Long-term (next quarter):**
   - [ ] Complete codebase migration
   - [ ] Promote linter to error severity
   - [ ] Add to contribution guidelines

## References

- [Error Handling Guide](../errors.md)
- [Error Handling Strategy PRD](error-handling-strategy.md)
- [Error Types and Sentinels](error-types-and-sentinels.md)
- [CockroachDB errors package](https://github.com/cockroachdb/errors)
- [Go errors.Is documentation](https://pkg.go.dev/errors#Is)

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-11-11 | System | Initial document for sentinel error enforcement strategy |
