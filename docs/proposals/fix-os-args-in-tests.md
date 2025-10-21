# Proposal: Replace os.Args with Cobra's SetArgs in Tests

## Problem

Many tests across the codebase are manipulating `os.Args` directly instead of using Cobra's proper `SetArgs()` method. This is an anti-pattern that causes several issues:

1. **Global State Pollution**: `os.Args` is global state that can leak between tests
2. **Not the Cobra Way**: Cobra provides `SetArgs()` specifically for testing
3. **Inconsistent with Best Practices**: Some tests use `SetArgs()`, others use `os.Args`
4. **Requires Manual Cleanup**: Every `os.Args` manipulation needs explicit defer/restore
5. **Makes Tests Fragile**: Tests can fail unpredictably when run in parallel

## Affected Files

Based on the audit, the following files use `os.Args` in tests:

### cmd/ package (7 files)
- `cmd/root_test.go` - 4 occurrences
- `cmd/cmd_utils_test.go` - 1 occurrence
- `cmd/auth_login_test.go` - 1 occurrence
- `cmd/terraform_test.go` - 3 occurrences

### tests/ package (3 files)
- `tests/validate_schema_test.go`
- `tests/describe_test.go`
- `tests/cli_describe_component_test.go`

### pkg/ package (1 file)
- `pkg/config/config_test.go` - 6 occurrences

### Root package integration tests (2 files)
- `main_plan_diff_integration_test.go` - 4 occurrences
- `main_hooks_and_store_integration_test.go` - 2 occurrences

### errors/ package (1 file)
- `errors/error_funcs_test.go` - 4 occurrences (legitimate use for subprocess testing)

**Total: 14 files with ~26 occurrences of os.Args manipulation**

## Analysis

### Categories of Usage

1. **Command Execution Tests** (should use SetArgs):
   - `cmd/root_test.go`: `os.Args = []string{"atmos", "about"}`
   - `tests/cli_describe_component_test.go`: `os.Args = []string{"atmos", "describe", "component", ...}`
   - `main_plan_diff_integration_test.go`: `os.Args = []string{"atmos", "terraform", "plan", ...}`

2. **Flag Parsing Tests** (should use SetArgs):
   - `pkg/config/config_test.go`: Testing `parseFlags()` function
   - `cmd/cmd_utils_test.go`: Testing `isVersionCommand()`

3. **Subprocess Exit Testing** (legitimate os.Args usage):
   - `errors/error_funcs_test.go`: Uses `os.Args[0]` to spawn subprocess for exit code testing
   - This is **correct usage** - needs to fork test binary

4. **Integration Tests Calling main()** (needs special handling):
   - `main_hooks_and_store_integration_test.go`: Calls `main()` directly
   - `main_plan_diff_integration_test.go`: Calls `main()` directly
   - These need `os.Args` because they're testing the `main()` function entry point

## Proposed Solution

### Phase 1: Fix cmd/ Package Tests (Highest Priority)

These are the easiest and most important to fix:

#### Pattern A: Tests using Execute() or RootCmd.Execute()

**Before:**
```go
func TestSomething(t *testing.T) {
    oldArgs := os.Args
    defer func() { os.Args = oldArgs }()
    os.Args = []string{"atmos", "terraform", "plan"}

    err := Execute()
    // assertions...
}
```

**After:**
```go
func TestSomething(t *testing.T) {
    t := cmd.NewTestKit(t)  // Already handles RootCmd cleanup

    RootCmd.SetArgs([]string{"terraform", "plan"})  // No "atmos" prefix!

    err := Execute()
    // assertions...
}
```

**Key Changes:**
- Use `cmd.NewTestKit(t)` for automatic RootCmd cleanup
- Use `RootCmd.SetArgs()` instead of `os.Args`
- Remove "atmos" from args (Cobra handles the program name)
- No manual defer/restore needed

#### Files to Update:
- `cmd/root_test.go`: 4 tests
- `cmd/auth_login_test.go`: 1 test (already uses TestKit, just needs SetArgs)

### Phase 2: Fix pkg/config Tests

The `pkg/config/config_test.go` tests are testing `parseFlags()` which internally uses `os.Args`. Two options:

#### Option A: Refactor parseFlags() to Accept Args
```go
// Before
func parseFlags() map[string]string {
    flags := make(map[string]string)
    for _, arg := range os.Args[1:] {
        // parse flags
    }
    return flags
}

// After
func parseFlags() map[string]string {
    return parseFlagsFromArgs(os.Args)
}

func parseFlagsFromArgs(args []string) map[string]string {
    flags := make(map[string]string)
    for _, arg := range args[1:] {
        // parse flags
    }
    return flags
}
```

Then tests call `parseFlagsFromArgs()` directly with test args.

#### Option B: Keep os.Args but Document as Legacy
If `parseFlags()` is internal and not widely used, document that these tests require `os.Args` manipulation for legacy reasons.

### Phase 3: Fix tests/ Package Tests

Tests in `tests/` that call `cmd.Execute()` should follow the same pattern as cmd/ tests:

**Before:**
```go
func TestExecuteDescribeComponentCmd_Success_YAMLWithPager(t *testing.T) {
    t.Chdir("./fixtures/scenarios/atmos-include-yaml-function")
    oldArgs := os.Args
    defer func() { os.Args = oldArgs }()
    os.Args = []string{"atmos", "describe", "component", "component-1", "--stack", "nonprod"}
    if err := cmd.Execute(); err != nil {
        t.Fatalf("failed to execute command: %v", err)
    }
}
```

**After:**
```go
func TestExecuteDescribeComponentCmd_Success_YAMLWithPager(t *testing.T) {
    t.Chdir("./fixtures/scenarios/atmos-include-yaml-function")

    // Import cmd package to access NewTestKit and RootCmd
    // (May need to refactor to avoid import cycle)
    cmd.RootCmd.SetArgs([]string{"describe", "component", "component-1", "--stack", "nonprod"})

    if err := cmd.Execute(); err != nil {
        t.Fatalf("failed to execute command: %v", err)
    }
}
```

**Challenge**: The `tests/` package may have import cycle issues with `cmd/`. Solutions:
1. Move these tests to `cmd/*_test.go` files
2. Export a test helper in `cmd/` for setting args
3. Keep as-is if import cycles prevent refactoring

### Phase 4: Document Legitimate os.Args Usage

Some uses of `os.Args` are **correct** and should be kept:

#### 1. Subprocess Testing (errors/error_funcs_test.go)
```go
// This is CORRECT - testing exit codes requires subprocess
execPath, err := exec.LookPath(os.Args[0])
cmd := exec.Command(execPath, "-test.run=TestFoo")
```

**Action**: Add comment explaining why `os.Args[0]` is used:
```go
// Use os.Args[0] to get path to test binary for subprocess execution.
// This is the correct pattern for testing exit codes.
execPath, err := exec.LookPath(os.Args[0])
```

#### 2. Integration Tests Calling main()
Files like `main_hooks_and_store_integration_test.go` that call `main()` directly **need** `os.Args`:

```go
// This is CORRECT - main() reads os.Args directly
func TestHooksAndStore(t *testing.T) {
    origArgs := os.Args
    defer func() { os.Args = origArgs }()

    os.Args = []string{"atmos", "terraform", "deploy", "component1", "-s", "test"}
    main()  // main() has no parameters, must read os.Args
}
```

**Action**: Add comment explaining:
```go
// Use os.Args here because we're testing main() directly.
// main() has no parameters and reads os.Args internally.
origArgs := os.Args
defer func() { os.Args = origArgs }()
```

### Phase 5: Update CLAUDE.md Guidelines

Add section to CLAUDE.md:

```markdown
### Testing Command Execution (MANDATORY)
- **ALWAYS use `RootCmd.SetArgs()` for Cobra command tests** - Never manipulate `os.Args`
- **Use `cmd.NewTestKit(t)` for automatic cleanup** - Handles RootCmd state reset
- **Remove program name from args** - Cobra handles this automatically
- **Examples**:
  ```go
  // ❌ WRONG: Using os.Args
  func TestCommand(t *testing.T) {
      oldArgs := os.Args
      defer func() { os.Args = oldArgs }()
      os.Args = []string{"atmos", "terraform", "plan"}
      Execute()
  }

  // ✅ CORRECT: Using SetArgs
  func TestCommand(t *testing.T) {
      t := cmd.NewTestKit(t)  // Auto cleanup
      cmd.RootCmd.SetArgs([]string{"terraform", "plan"})
      Execute()
  }
  ```

- **Exceptions** (legitimate os.Args usage):
  1. Subprocess testing: `exec.LookPath(os.Args[0])` to spawn test binary
  2. Testing main(): When testing main() entry point directly
  3. Always document why os.Args is needed with a comment
```

## Implementation Plan

### Priority Order

1. **High Priority**: Fix `cmd/` package tests (6 files)
   - These are core command tests
   - Easy to fix with existing TestKit
   - High impact on test reliability

2. **Medium Priority**: Fix `pkg/config/` tests (1 file)
   - Requires refactoring parseFlags()
   - 6 test functions affected
   - Improves config package testability

3. **Low Priority**: Fix `tests/` package (3 files)
   - May have import cycle issues
   - Could require test reorganization
   - Less critical as integration tests

4. **Documentation**: Add comments to legitimate uses (3 files)
   - errors/error_funcs_test.go
   - main_*_integration_test.go (2 files)
   - Update CLAUDE.md

### Estimated Effort

- Phase 1 (cmd/ tests): 2-3 hours
- Phase 2 (pkg/config tests): 1-2 hours
- Phase 3 (tests/ package): 2-4 hours (may hit import issues)
- Phase 4 (documentation): 30 minutes
- Phase 5 (CLAUDE.md): 30 minutes

**Total: 6-10 hours**

### Success Criteria

1. All `cmd/` package tests use `SetArgs()` instead of `os.Args`
2. `pkg/config` tests either refactored or documented as legacy
3. Legitimate `os.Args` usage documented with explanatory comments
4. CLAUDE.md updated with testing guidelines
5. All tests pass with `go test ./...`
6. No test pollution when running tests in parallel

### Risks & Mitigation

**Risk 1: Import Cycles**
- `tests/` package may have cycles with `cmd/`
- **Mitigation**: Move tests to `cmd/` or use internal test helpers

**Risk 2: Breaking Integration Tests**
- Tests calling `main()` need different approach
- **Mitigation**: Document as exception, keep `os.Args` usage

**Risk 3: Test Coverage Drop**
- Refactoring might temporarily break tests
- **Mitigation**: Fix tests one file at a time, run suite after each

## Questions for Review

1. Should we prioritize all phases or just focus on Phase 1 (cmd/ package)?
2. For `pkg/config/config_test.go`, prefer Option A (refactor) or B (document)?
3. Should we move `tests/` integration tests to `cmd/` to avoid import cycles?
4. Any other testing patterns that should be standardized?

## References

- **Cobra Testing Docs**: https://github.com/spf13/cobra/blob/main/user_guide.md#testing
- **CLAUDE.md Test Isolation**: Lines discussing TestKit usage
- **Go Testing Best Practices**: https://go.dev/wiki/TestComments
