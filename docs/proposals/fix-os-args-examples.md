# Fix os.Args in Tests - Specific Examples

This document provides concrete before/after examples for each affected test file.

## Summary of Changes Needed

| File | Occurrences | Difficulty | Notes |
|------|-------------|------------|-------|
| cmd/root_test.go | 4 | Easy | Already uses TestKit in some tests |
| cmd/cmd_utils_test.go | 1 | Easy | Simple fix |
| cmd/auth_login_test.go | 1 | Easy | Already uses TestKit |
| cmd/terraform_test.go | 3 | Easy | Subprocess testing - add comments only |
| tests/validate_schema_test.go | ? | Medium | Check for cmd.Execute() calls |
| tests/describe_test.go | ? | Medium | Check for cmd.Execute() calls |
| tests/cli_describe_component_test.go | 1 | Medium | May have import issues |
| pkg/config/config_test.go | 6 | Medium | Requires parseFlags() refactor |
| main_plan_diff_integration_test.go | 4 | Document | Testing main() - keep os.Args |
| main_hooks_and_store_integration_test.go | 2 | Document | Testing main() - keep os.Args |
| errors/error_funcs_test.go | 4 | Document | Subprocess testing - keep os.Args |

## Detailed Examples

### 1. cmd/root_test.go:TestNoColorLog

**Current Code (Lines 57-62):**
```go
oldArgs := os.Args
defer func() {
    os.Args = oldArgs
}()
// Set the arguments for the command
os.Args = []string{"atmos", "about"}
```

**Fixed Code:**
```go
// Use SetArgs - no defer needed, TestKit handles cleanup
RootCmd.SetArgs([]string{"about"})
```

**Note**: This test already uses `NewTestKit(t)` on line 25, so cleanup is automatic.

---

### 2. cmd/root_test.go:TestInitFunction

**Current Code (Lines 81-92):**
```go
// Save the original state
originalArgs := os.Args
originalEnvVars := make(map[string]string)
defer func() {
    // Restore original state
    os.Args = originalArgs
    for k, v := range originalEnvVars {
        if v == "" {
            os.Unsetenv(k)
        } else {
            os.Setenv(k, v)
        }
    }
}()
```

**Fixed Code:**
```go
// No os.Args manipulation needed in this test
// If needed for subtests, each subtest should set args via SetArgs
```

**Note**: This test doesn't actually set `os.Args`, it just saves/restores. The defer is unnecessary - remove it.

---

### 3. cmd/root_test.go:TestIsCompletionCommand

**Current Code (Lines 462-466):**
```go
// Save original args.
originalArgs := os.Args
defer func() {
    os.Args = originalArgs
}()
```

**Fixed Code:**
```go
// Remove the defer - test doesn't actually modify os.Args
// The test checks environment variables, not os.Args
```

**Note**: This test saves `os.Args` but never modifies it. The defer is defensive but unnecessary.

---

### 4. cmd/cmd_utils_test.go:TestIsVersionCommand

**Current Code (Lines 174-179):**
```go
// Save original os.Args
oldArgs := os.Args
defer func() { os.Args = oldArgs }()

// Set up test args
os.Args = append([]string{"atmos"}, tt.args...)
```

**Fixed Code:**
```go
// Test isVersionCommand() with different args
// Option 1: Refactor isVersionCommand() to accept args parameter
result := isVersionCommand(append([]string{"atmos"}, tt.args...))

// Option 2: If isVersionCommand() must use os.Args, document it
// isVersionCommand() is a legacy function that reads os.Args directly.
// Keep os.Args manipulation but add this comment.
oldArgs := os.Args
defer func() { os.Args = oldArgs }()
os.Args = append([]string{"atmos"}, tt.args...)
```

**Recommendation**: Check if `isVersionCommand()` can be refactored to accept args as parameter.

---

### 5. cmd/auth_login_test.go

**Current Code (Lines 131-132):**
```go
originalArgs := os.Args
defer func() { os.Args = originalArgs }()
```

**Fixed Code:**
```go
// Remove entirely - test doesn't use os.Args
// Test already uses NewTestKit(t) on line 128
// Test creates its own cobra.Command and uses cmd.SetArgs()
```

**Note**: This test already uses `cmd.SetArgs()` on line 177, so the os.Args handling is unnecessary defensive code.

---

### 6. tests/cli_describe_component_test.go

**Current Code (Lines 12-14):**
```go
oldArgs := os.Args
defer func() { os.Args = oldArgs }()
os.Args = []string{"atmos", "describe", "component", "component-1", "--stack", "nonprod", "--pager=more", "--format", "yaml"}
```

**Fixed Code - Option A (if no import cycle):**
```go
// Use cmd package functions (check for import cycles first)
cmd.RootCmd.SetArgs([]string{"describe", "component", "component-1", "--stack", "nonprod", "--pager=more", "--format", "yaml"})
```

**Fixed Code - Option B (if import cycle exists):**
```go
// Keep os.Args but add comment explaining limitation
// tests/ package imports cmd, so we can't use cmd.NewTestKit without cycle.
// Using os.Args here as tests/ is for integration testing.
oldArgs := os.Args
defer func() { os.Args = oldArgs }()
os.Args = []string{"atmos", "describe", "component", "component-1", "--stack", "nonprod", "--pager=more", "--format", "yaml"}
```

---

### 7. pkg/config/config_test.go - Multiple Tests

**Current Pattern (repeated 6 times):**
```go
oldArgs := os.Args
defer func() { os.Args = oldArgs }()
os.Args = test.args
```

**Option A: Refactor parseFlags() (Recommended)**

Add new exported test helper:
```go
// In pkg/config/config.go:
func parseFlags() map[string]string {
    return ParseFlagsFromArgs(os.Args)
}

// ParseFlagsFromArgs parses flags from the given args slice.
// Exposed for testing purposes.
func ParseFlagsFromArgs(args []string) map[string]string {
    flags := make(map[string]string)
    for i := 1; i < len(args); i++ {
        arg := args[i]
        // ... existing parsing logic ...
    }
    return flags
}
```

Then update tests:
```go
// Before
oldArgs := os.Args
defer func() { os.Args = oldArgs }()
os.Args = test.args
result := parseFlags()

// After
result := ParseFlagsFromArgs(test.args)
```

**Option B: Document as Legacy**
```go
// parseFlags() is a legacy function that reads os.Args directly.
// Tests must manipulate os.Args to test this function.
oldArgs := os.Args
defer func() { os.Args = oldArgs }()
os.Args = test.args
result := parseFlags()
```

---

### 8. errors/error_funcs_test.go - Subprocess Testing (KEEP AS-IS)

**Current Code (Lines 51, 70, 96, 128):**
```go
execPath, err := exec.LookPath(os.Args[0])
```

**Add Comment:**
```go
// Use os.Args[0] to get path to test binary for subprocess execution.
// This is the correct pattern for testing os.Exit behavior.
execPath, err := exec.LookPath(os.Args[0])
```

**Reason**: These tests spawn subprocesses to test exit codes. Using `os.Args[0]` is the **correct** way to get the test binary path.

---

### 9. main_plan_diff_integration_test.go (KEEP AS-IS)

**Current Code (Lines 68-69, 78, 86, 94, 116):**
```go
origArgs := os.Args
defer func() { os.Args = origArgs }()

os.Args = []string{"atmos", "terraform", "plan", "component-1", "-s", "nonprod", "-out=" + origPlanFile}
exitCode := runMainWithExitCode()
```

**Add Comment:**
```go
// This test calls main() directly which reads os.Args.
// Using os.Args is necessary for integration testing the main() entry point.
origArgs := os.Args
defer func() { os.Args = origArgs }()

os.Args = []string{"atmos", "terraform", "plan", "component-1", "-s", "nonprod", "-out=" + origPlanFile}
exitCode := runMainWithExitCode()
```

**Reason**: Testing `main()` function requires `os.Args` because main() has no parameters.

---

### 10. main_hooks_and_store_integration_test.go (KEEP AS-IS)

**Current Code (Lines 29-30, 34, 39):**
```go
// Capture the original arguments
origArgs := os.Args
defer func() { os.Args = origArgs }()

os.Args = []string{"atmos", "terraform", "deploy", "component1", "-s", "test"}
main()
```

**Add Comment:**
```go
// This integration test calls main() directly which reads os.Args internally.
// Using os.Args is necessary for testing the complete main() execution path.
origArgs := os.Args
defer func() { os.Args = origArgs }()

os.Args = []string{"atmos", "terraform", "deploy", "component1", "-s", "test"}
main()
```

**Reason**: Testing `main()` function requires `os.Args` because main() has no parameters.

---

## Implementation Checklist

### Phase 1: Easy Fixes (cmd/ package)
- [ ] Fix `cmd/root_test.go:TestNoColorLog` - use SetArgs
- [ ] Fix `cmd/root_test.go:TestInitFunction` - remove unnecessary defer
- [ ] Fix `cmd/root_test.go:TestIsCompletionCommand` - remove unnecessary defer
- [ ] Fix `cmd/auth_login_test.go` - remove unnecessary defer
- [ ] Review `cmd/cmd_utils_test.go:TestIsVersionCommand` - refactor or document

### Phase 2: Medium Fixes
- [ ] Fix `pkg/config/config_test.go` - refactor parseFlags() or document
- [ ] Check `tests/validate_schema_test.go` - fix if uses cmd.Execute()
- [ ] Check `tests/describe_test.go` - fix if uses cmd.Execute()
- [ ] Fix `tests/cli_describe_component_test.go` - refactor or document import cycle

### Phase 3: Documentation
- [ ] Add comment to `errors/error_funcs_test.go` (4 locations)
- [ ] Add comment to `main_plan_diff_integration_test.go` (5 locations)
- [ ] Add comment to `main_hooks_and_store_integration_test.go` (2 locations)

### Phase 4: Guidelines
- [ ] Update CLAUDE.md with testing best practices
- [ ] Add examples of correct vs incorrect patterns
- [ ] Document exceptions for subprocess and main() testing

---

## Testing Strategy

After making changes to each file:

```bash
# Test the specific file
go test ./cmd -v -run TestNoColorLog
go test ./pkg/config -v -run TestParseFlags

# Run full test suite
go test ./...

# Run tests in parallel to catch state pollution
go test -parallel 10 ./cmd/...
```

## Questions Before Implementation

1. **Import Cycles**: Should we check `tests/` package for import cycles before deciding on fix approach?
2. **parseFlags() Refactor**: Is Option A (expose ParseFlagsFromArgs) preferred over Option B (document as legacy)?
3. **Priority**: Should we fix everything in one PR, or split into multiple PRs (cmd/, pkg/, docs)?
4. **Backwards Compatibility**: Any concerns about refactoring parseFlags() affecting other code?
