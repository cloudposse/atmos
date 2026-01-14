# PRD: Eliminating Deep Exits from Atmos

## Problem Statement

Deep exits occur when code calls `os.Exit()`, `log.Fatal()`, or `log.Panic()` deep within the call stack, terminating the program immediately without allowing proper error handling. This anti-pattern has been used throughout Atmos and creates significant problems for code quality, testability, and maintainability.

With the introduction of our command registry, flag registry, improved error handling infrastructure, and architectural patterns, we now have the foundation to systematically eliminate deep exits and move toward cleaner, more testable code.

## Why Deep Exits Are Problematic

### 1. Testability
- **Cannot test error paths** - Tests that trigger `os.Exit()` terminate the test process
- **Forces integration tests** - Unit tests become impossible, requiring slower integration tests
- **No error verification** - Cannot assert on error messages or behavior
- **Test isolation breaks** - One test can crash the entire test suite

### 2. Error Handling
- **Bypasses Go idioms** - Violates idiomatic Go error handling with explicit returns
- **Prevents error context** - Cannot wrap errors with additional context up the stack
- **Breaks error chains** - `errors.Is()` and `errors.As()` cannot work
- **No recovery possible** - Calling code cannot handle errors gracefully

### 3. Command Registry Integration
- **Inconsistent behavior** - Registry cannot handle errors uniformly across commands
- **Telemetry gaps** - Error telemetry cannot capture exit events
- **UI inconsistency** - Cannot display errors through standard UI layer
- **No error middleware** - Cannot add cross-cutting error handling concerns

### 4. Composability
- **Cannot compose commands** - Commands cannot call other commands programmatically
- **Workflow limitations** - Workflows cannot handle command failures gracefully
- **Library usage impossible** - Code cannot be used as a library
- **CI/CD brittleness** - Cannot distinguish error types or exit codes properly

### 5. Code Quality
- **Violates separation of concerns** - Business logic shouldn't control program lifecycle
- **Hidden control flow** - Exit points are not obvious from function signatures
- **Debugging difficulty** - Stack traces end at the exit point
- **Maintenance burden** - Changes to error handling require finding all exit points

## Current State

Deep exits are prevalent throughout Atmos:
- `log.Fatal()` - Used in numerous places across `cmd/` and `internal/exec/`
- `os.Exit()` - Called directly from non-main functions
- `log.Panic()` - Used in error scenarios
- Helper functions like `CheckErrorPrintAndExit()` - Encapsulate deep exits

Example from `internal/exec/version.go:54-57`:
```go
err := v.printStyledText("ATMOS")
if err != nil {
    //nolint:revive // deep-exit: log.Fatal is appropriate here
    log.Fatal(err)  // ❌ Terminates process immediately
}
```

## Proposed Solution

Eliminate all deep exits by returning errors through the normal Go error handling flow, allowing `main()` and top-level command handlers to be the only places that call `os.Exit()`.

### Phase 1: Simple Error Returns (Immediate - No Dependencies)

Replace deep exits with standard error returns using `fmt.Errorf()` for wrapping and context.

**Before:**
```go
func (v versionExec) Execute(checkFlag bool, format string) error {
    err := v.printStyledText("ATMOS")
    if err != nil {
        log.Fatal(err)  // ❌ Deep exit
    }
    // ... more code
    return nil
}
```

**After:**
```go
func (v versionExec) Execute(checkFlag bool, format string) error {
    err := v.printStyledText("ATMOS")
    if err != nil {
        return fmt.Errorf("failed to display styled text: %w", err)  // ✅ Return error
    }
    // ... more code
    return nil
}
```

**Benefits:**
- Works with existing infrastructure
- No dependencies on pending PRs
- Errors propagate to Cobra's `RunE` handler
- Fully testable with mocks
- Can add context at each layer

### Phase 2: Rich Error Builder (Future - After PR #1763)

Once error handling infrastructure from PR #1763 merges, enhance errors with hints, context, and better UX.

**Enhanced:**
```go
func (v versionExec) Execute(checkFlag bool, format string) error {
    err := v.printStyledText("ATMOS")
    if err != nil {
        return errUtils.Build(err).
            WithHint("Failed to display styled text").
            WithHint("Check terminal capabilities and encoding").
            WithExitCode(1).
            Err()
    }
    // ... more code
    return nil
}
```

**Additional Benefits:**
- Rich error messages with actionable hints
- Markdown-formatted error output
- Context tables in verbose mode
- Sentry integration for error tracking
- Consistent error UX across all commands

## Implementation Approach

### Reference Implementation

The version command serves as the reference implementation (this PR).

**Changes:**
1. Removed `log.Fatal(err)` deep exit
2. Replaced with `return fmt.Errorf("failed to display styled text: %w", err)`
3. All tests pass
4. Command works identically from user perspective

### Pattern for All Commands

**Step 1: Identify Deep Exits**
```bash
# Find all deep exits in a command
grep -r "log.Fatal" internal/exec/mycommand.go
grep -r "log.Panic" internal/exec/mycommand.go
grep -r "os.Exit" internal/exec/mycommand.go
grep -r "CheckErrorPrintAndExit" internal/exec/mycommand.go
```

**Step 2: Convert to Error Returns**
- Replace `log.Fatal(err)` → `return err` or `return fmt.Errorf("context: %w", err)`
- Replace `log.Fatalf(msg, args)` → `return fmt.Errorf(msg, args)`
- Replace `os.Exit(code)` → `return errors.New("reason")`
- Replace `CheckErrorPrintAndExit(err)` → `return err`

**Step 3: Update Function Signatures**
- Change `Run` → `RunE` in Cobra commands
- Change `PreRun` → `PreRunE` in Cobra commands
- Add `error` return to functions that didn't have one
- Propagate errors up the call stack

**Step 4: Add Error Context**
Use `%w` verb for error wrapping to maintain error chains:
```go
if err := someOperation(); err != nil {
    return fmt.Errorf("failed to perform operation: %w", err)
}
```

**Step 5: Test Error Paths**
Add test cases using dependency injection and mocks:
```go
func TestMyCommand_ErrorHandling(t *testing.T) {
    exec := &myExec{
        doWork: func() error {
            return errors.New("simulated failure")
        },
    }

    err := exec.Execute()
    require.Error(t, err)
    assert.Contains(t, err.Error(), "simulated failure")
}
```

### Command Registry Integration

Commands using the registry pattern get error handling for free:

```go
// cmd/mycommand/mycommand.go
var myCmd = &cobra.Command{
    Use: "mycommand",
    RunE: func(cmd *cobra.Command, args []string) error {
        opts, err := parseOptions(cmd, args)
        if err != nil {
            return err  // ✅ Cobra handles error display
        }

        return exec.NewMyCommandExec(atmosConfig).Execute(opts)  // ✅ Error propagates
    },
}
```

**Registry benefits:**
- Errors flow to Cobra automatically
- Telemetry captures all errors
- Exit codes are consistent
- UI displays errors uniformly

## Migration Strategy

### Phase 1: Foundation (Current - No Dependencies)

**Status:** ✅ Ready to implement

**Goals:**
- Establish pattern with version command (reference implementation)
- Document approach in this PRD
- Update CLAUDE.md with guidelines
- Enable linter rules to prevent regressions

**Deliverables:**
1. ✅ Version command refactored (this PR)
2. ✅ PRD documenting pattern
3. ⏳ Update CLAUDE.md
4. ⏳ Add linter enforcement

### Phase 2: Systematic Elimination

**Approach:** Incremental refactoring, one command per PR

**Prioritization:**
1. **High priority** - Frequently used commands (`terraform`, `describe`, `validate`)
2. **Medium priority** - Moderate usage (`helmfile`, `atlantis`, `workflow`)
3. **Low priority** - Infrequently used or legacy commands

**Process:**
1. Audit command for deep exits
2. Create focused PR for single command
3. Replace deep exits with error returns
4. Add test coverage for error paths
5. Verify no behavioral changes for users
6. Review and merge

**Tracking:**
- Create GitHub issues for each command
- Label with `refactoring`, `deep-exits`
- Track progress in project board

### Phase 3: Enhanced Error Handling (After PR #1763)

**Status:** ⏳ Blocked on PR #1763

**Goals:**
- Enhance errors with rich formatting
- Add context-aware hints
- Enable verbose mode with context tables
- Integrate Sentry for error tracking

**Approach:**
- Revisit refactored commands incrementally
- Add error builder enhancements
- Focus on user-facing errors first
- One command per PR for easy review

## Linter Enforcement

Add rules to `.golangci.yml` to prevent deep exits in new code:

```yaml
linters-settings:
  forbidigo:
    forbid:
      - pattern: os\.Exit
        msg: "Do not call os.Exit directly; return errors with exit codes using errUtils.WithExitCode() and let main() handle exit"
      - pattern: log\.Fatal
        msg: "Do not use log.Fatal; return errors and let main() handle exit"
      - pattern: log\.Panic
        msg: "Do not use log.Panic; return errors instead"
```

**Enforcement scope:**
- `internal/exec/` - All execution logic must return errors
- `cmd/` - Command handlers should use `RunE` and return errors
- Exceptions: `main.go`, `init()` functions only

## Testing Strategy

### Unit Tests with Mocks

Use dependency injection to test error paths:

```go
type commandExec struct {
    doWork func() error  // Injectable dependency
}

func TestCommandExec_ErrorHandling(t *testing.T) {
    tests := []struct {
        name        string
        doWork      func() error
        wantErr     bool
        errContains string
    }{
        {
            name:    "success",
            doWork:  func() error { return nil },
            wantErr: false,
        },
        {
            name:        "work fails",
            doWork:      func() error { return errors.New("work failed") },
            wantErr:     true,
            errContains: "work failed",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            exec := &commandExec{doWork: tt.doWork}
            err := exec.Execute()

            if tt.wantErr {
                require.Error(t, err)
                assert.Contains(t, err.Error(), tt.errContains)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

### Integration Tests

Verify command-level error handling:

```go
func TestCommandIntegration(t *testing.T) {
    cmd := NewTestKit(t)

    // Test invalid input returns error (not exit)
    err := cmd.Execute("mycommand", "--invalid-flag")
    require.Error(t, err)
    assert.Contains(t, err.Error(), "invalid flag")

    // Test error message is helpful
    assert.Contains(t, err.Error(), "unknown flag")
}
```

## Success Metrics

### Code Quality
- **Zero deep exits** in `internal/exec/` (except whitelisted cases)
- **100% test coverage** on error paths for refactored commands
- **All tests passing** with no behavioral changes

### Developer Experience
- **Easier testing** - Error paths can be tested with simple mocks
- **Better debugging** - Full stack traces available
- **Clearer code** - Error handling is explicit in function signatures

### User Experience
- **No changes** - Commands work identically from user perspective
- **Better errors** (Phase 2) - Rich error messages with hints and context
- **Consistent UX** - All commands handle errors uniformly

## References

- **PR #1606** - Deep exits elimination (reference implementation)
  - Eliminated 116 deep exit instances
  - Added 91 test cases, achieved 100% coverage
  - Used simple error returns (Phase 1 approach)

- **PR #1763** - Error handling infrastructure (pending)
  - Error builder with fluent API
  - Markdown-formatted errors
  - Sentry integration
  - Verbose mode with context tables

- **Command Registry Pattern** - `docs/prd/command-registry-pattern.md`
- **Error Handling Strategy** - `docs/prd/error-handling-strategy.md`
- **Testing Strategy** - `docs/prd/testing-strategy.md`

## Next Steps

1. ✅ Complete version command refactoring (this PR)
2. Update CLAUDE.md with deep-exit avoidance guidelines
3. Add linter rules to `.golangci.yml`
4. Create GitHub issues for remaining commands with deep exits
5. Begin Phase 2 incremental refactoring (one command per PR)
6. Monitor PR #1763 for merge to enable Phase 3 enhancements
