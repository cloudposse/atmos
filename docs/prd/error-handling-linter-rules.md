# Error Handling Linter Rules

This document explains the linting strategy for error handling patterns in Atmos, addressing consistency and automated enforcement.

## Current State

### Multiple `%w` in fmt.Errorf

**Status:** ✅ Valid in Go 1.20+ (Atmos uses Go 1.24.8)

Go 1.20 added support for multiple `%w` verbs in `fmt.Errorf`:
```go
// Valid Go 1.20+ - returns error with Unwrap() []error
err := fmt.Errorf("%w: failed to do X: %w", errUtils.ErrBase, underlyingErr)
```

**Current Linting:** Already enforced by `errorlint` with `errorf-multi: true` in `.golangci.yml`:
```yaml
errorlint:
  errorf: true         # Enforce %w over %v for errors
  errorf-multi: true   # Support multiple %w (Go 1.20+)
  asserts: true        # Suggest errors.As for type assertions
  comparison: true     # Suggest errors.Is for error comparisons
```

### Error Wrapping Consistency

**Problem:** Inconsistent patterns across the codebase:

**Pattern 1: Nested `fmt.Errorf` with multiple `%w`**
```go
// pkg/auth/identities/aws/permission_set.go
return fmt.Errorf("%w: failed to setup AWS files: %w", errUtils.ErrAwsAuth, err)
```

**Pattern 2: `errors.Join` with base error**
```go
// pkg/auth/identities/aws/user.go
return errors.Join(errUtils.ErrAwsAuth, err)
```

**Pattern 3: `fmt.Errorf` with single `%w` and context**
```go
return fmt.Errorf("%w: additional context", errUtils.ErrAwsAuth)
```

## Recommended Patterns

### When to Use Each Pattern

1. **`errors.Join`** - Multiple independent errors to preserve:
   ```go
   // Both errors are equally important, no descriptive message needed
   return errors.Join(errUtils.ErrAwsAuth, err)

   // Multiple unrelated errors
   return errors.Join(validationErr, configErr, fileErr)
   ```

2. **`fmt.Errorf` with single `%w`** - Add descriptive context:
   ```go
   // Add specific context about what failed
   return fmt.Errorf("%w: failed to authenticate with role %q", errUtils.ErrAwsAuth, roleArn)

   // Wrap with additional information
   return fmt.Errorf("component %s: %w", component, err)
   ```

3. **`fmt.Errorf` with multiple `%w`** - Chain multiple errors with context:
   ```go
   // Valid Go 1.20+ - preserves both errors in chain
   return fmt.Errorf("%w: identity %q: %w", errUtils.ErrAuthenticationFailed, name, err)
   ```

### Consistency Guidelines

For Atmos codebase consistency, prefer:

1. **Simple wrapping without extra context:** Use `errors.Join`
   ```go
   // PREFER
   return errors.Join(errUtils.ErrAwsAuth, err)

   // AVOID (redundant "failed to" adds no value)
   return fmt.Errorf("%w: failed to setup files: %w", errUtils.ErrAwsAuth, err)
   ```

2. **Wrapping with valuable context:** Use `fmt.Errorf` with single `%w`
   ```go
   // GOOD - adds specific context
   return fmt.Errorf("%w: role=%s region=%s", errUtils.ErrAwsAuth, role, region)

   // GOOD - specific operation failed
   return fmt.Errorf("failed to create IAM client: %w", err)
   ```

3. **Multiple error wrapping:** Use `fmt.Errorf` with multiple `%w` sparingly
   ```go
   // Use only when you need both errors preserved AND context string
   return fmt.Errorf("%w: authentication failed for %q: %w",
       errUtils.ErrAuthenticationFailed, identityName, err)
   ```

## Proposed Linter Rules

### Option 1: Add Custom Lintroller Rules

Add to `tools/lintroller/rule_error_wrapping.go`:

```go
package lintroller

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// ErrorWrappingRule checks for redundant error wrapping patterns.
type ErrorWrappingRule struct{}

func (r *ErrorWrappingRule) Check(pass *analysis.Pass) {
	inspect := func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check for fmt.Errorf calls
		if !isFmtErrorf(call, pass) {
			return true
		}

		// Check for redundant wrapping: fmt.Errorf("%w: failed to X: %w", ...)
		if hasRedundantWrapping(call) {
			pass.Reportf(call.Pos(),
				"redundant error wrapping: use errors.Join instead of fmt.Errorf with generic 'failed to' message")
		}

		return true
	}

	for _, file := range pass.Files {
		ast.Inspect(file, inspect)
	}
}

func hasRedundantWrapping(call *ast.CallExpr) bool {
	// Check if format string contains pattern: "%w: failed to ...: %w"
	// This is redundant - should use errors.Join instead
	if len(call.Args) < 1 {
		return false
	}

	formatLit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || formatLit.Kind != token.STRING {
		return false
	}

	format := formatLit.Value

	// Detect patterns like "%w: failed to X: %w" with no specific context
	if strings.Contains(format, "%w") &&
	   strings.Count(format, "%w") == 2 &&
	   (strings.Contains(format, "failed to") || strings.Contains(format, "error")) {
		// Check if there's actual context between the %w verbs
		// If it's just generic "failed to X", suggest errors.Join
		return true
	}

	return false
}
```

Configuration in `.golangci.yml`:
```yaml
custom:
  lintroller:
    type: "module"
    description: "Atmos project-specific linting rules"
    settings:
      tsetenv-in-defer: true
      os-setenv-in-test: true
      os-mkdirtemp-in-test: true
      redundant-error-wrapping: true  # NEW: Detect redundant fmt.Errorf with generic messages
```

### Option 2: Use `gocritic` Rules

The `gocritic` linter has some error handling checks. Add to `.golangci.yml`:

```yaml
gocritic:
  enabled-checks:
    # ... existing checks
    - wrapperFunc      # Detects functions that only wrap other functions
    - unnecessaryBlock # Detects unnecessary code blocks
```

However, `gocritic` doesn't have specific rules for `errors.Join` vs `fmt.Errorf` patterns.

### Option 3: Use `revive` Custom Rules

Add a custom `revive` rule in `.golangci.yml`:

```yaml
revive:
  rules:
    # ... existing rules
    - name: error-naming
      disabled: false
    - name: error-return
      disabled: false
```

However, `revive` also lacks specific `errors.Join` enforcement.

## Recommendation

**Implement Option 1** - Add custom lintroller rules for Atmos-specific patterns:

1. **`redundant-error-wrapping` rule:**
   - Detect `fmt.Errorf("%w: failed to X: %w", base, err)` patterns
   - Suggest `errors.Join(base, err)` when no specific context is added
   - Allow `fmt.Errorf` when format string contains variables or specific context

2. **`prefer-errors-join` rule:**
   - Detect simple double-wrapping: `fmt.Errorf("%w: %w", err1, err2)`
   - Suggest `errors.Join(err1, err2)` for clarity

3. **Benefits:**
   - Enforces Atmos coding standards automatically
   - Catches issues at CI time, not code review time
   - Educates developers through linter messages
   - Fully customizable to project needs

## Migration Path

For existing codebase:

1. **Phase 1:** Add linter rules as warnings (not errors)
   ```yaml
   severity:
     rules:
       - linters:
           - lintroller
         text: "redundant-error-wrapping|prefer-errors-join"
         severity: warning
   ```

2. **Phase 2:** Fix existing violations incrementally
   - Run `make lint` to identify all violations
   - Create tracking issue for cleanup
   - Fix violations in batches by package

3. **Phase 3:** Promote to errors after cleanup
   - Change severity from `warning` to `error`
   - Enforce for all new code

## Examples

### ❌ Redundant Pattern (Linter Would Flag)
```go
// Adds no specific context, just restates that it failed
return fmt.Errorf("%w: failed to setup AWS files: %w", errUtils.ErrAwsAuth, err)
return fmt.Errorf("%w: error setting auth context: %w", errUtils.ErrAwsAuth, err)
```

### ✅ Better Pattern
```go
// Use errors.Join when no additional context
return errors.Join(errUtils.ErrAwsAuth, err)

// Or add specific context with single %w
return fmt.Errorf("%w: role=%s region=%s", errUtils.ErrAwsAuth, roleArn, region)
```

### ✅ Valid Multiple %w Pattern
```go
// Provides specific context (identity name) between errors
return fmt.Errorf("%w: identity %q authentication failed: %w",
    errUtils.ErrAuthenticationFailed, identityName, err)
```

## Linter Implementation Checklist

- [ ] Create `tools/lintroller/rule_error_wrapping.go`
- [ ] Implement `redundant-error-wrapping` rule
- [ ] Add tests in `tools/lintroller/lintroller_test.go`
- [ ] Update `.golangci.yml` with new rule setting
- [ ] Add documentation to this PRD
- [ ] Create migration tracking issue
- [ ] Add examples to `CLAUDE.md`
- [ ] Run `golangci-lint custom` to rebuild `custom-gcl` binary
- [ ] Test on CI with warning severity
- [ ] Fix existing violations
- [ ] Promote to error severity

## References

- [Go 1.20 Release Notes - Multiple %w](https://go.dev/doc/go1.20#errors)
- [errorlint Documentation](https://github.com/polyfloyd/go-errorlint)
- [golangci-lint Module Plugins](https://golangci-lint.run/docs/plugins/module-plugins/)
- [Atmos Error Handling Strategy](./error-handling-strategy.md)
