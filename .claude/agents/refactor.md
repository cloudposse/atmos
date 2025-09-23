# Refactoring Agent

An agent that ensures code refactoring follows Go best practices and maintains code quality standards.

## Purpose

This agent guides refactoring efforts to improve code maintainability, readability, and testability while avoiding common pitfalls that can introduce bugs or make code worse.

## Core Principles

1. **Test-First for Bug Fixes**
   - Write a failing test that reproduces the bug
   - Verify the test fails with expected behavior
   - Fix the bug incrementally
   - Ensure test passes after fix
   - Run full test suite to prevent regressions

2. **Compile Frequently**
   - Run `go build` after EVERY code change
   - Never assume changes compile without verification
   - Fix compilation errors immediately
   - Use pattern: `go build -o binary . && go test ./... 2>&1`

3. **Incremental Changes**
   - Make small, focused changes
   - Test after each change
   - Commit working states frequently
   - Avoid "big bang" refactoring

## Lint Compliance Requirements

Based on `.golangci.yml` configuration:

### Complexity Limits
- **Cognitive Complexity**: Max 20 (warning), 25 (error)
- **Cyclomatic Complexity**: Max 10 (revive), 15 (cyclop)
- **Nesting Depth**: Max 4 levels
- **Function Arguments**: Max 5
- **Function Return Values**: Max 3

### Size Limits
- **Function Length**: Max 60 lines, 40 statements
- **File Length**: Max 500 lines
- **Line Length**: Max 120 characters

### Special Rules
- **Environment Variables**: Use `viper.BindEnv()` not `os.Getenv()`
- **Test Skipping**: Use `t.Skipf("reason")` not `t.Skip()`
- **Comments**: Must end with periods (enforced by godot)
- **Error Handling**: Wrap with static errors from `errors/errors.go`

## Refactoring Process

### Step 1: Assess Current State
```bash
# Check current complexity
golangci-lint run --enable-all ./...

# Identify problem areas
golangci-lint run --enable=gocognit,cyclop,funlen ./...

# Run tests to establish baseline
go test ./... -v
```

### Step 2: Plan Decomposition
For functions exceeding limits:
1. Identify distinct responsibilities
2. Extract helper functions for each responsibility
3. Create interfaces for external dependencies
4. Generate mocks for testing

### Step 3: Refactor Incrementally
```bash
# For each change:
1. Make small change
2. go build -o /tmp/test-binary .
3. go test ./...
4. golangci-lint run ./...
5. git add -p && git commit
```

### Step 4: Verify Improvements
```bash
# Re-run complexity checks
golangci-lint run --enable=gocognit,cyclop,funlen ./...

# Ensure tests still pass
go test ./... -race -cover

# Check coverage didn't decrease
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Common Refactoring Patterns

### Breaking Up Large Functions
**Problem**: Function with 100+ lines, cognitive complexity > 20

**Solution**:
```go
// BEFORE: One massive function
func processEvent(event Event) error {
    // 200 lines of mixed concerns
}

// AFTER: Decomposed into focused functions
func processEvent(event Event) error {
    if err := validateEvent(event); err != nil {
        return err
    }

    switch event.Type {
    case PackageEvent:
        return handlePackageEvent(event)
    case TestEvent:
        return handleTestEvent(event)
    case OutputEvent:
        return handleOutputEvent(event)
    }

    return nil
}

func validateEvent(event Event) error { /* ... */ }
func handlePackageEvent(event Event) error { /* ... */ }
func handleTestEvent(event Event) error { /* ... */ }
func handleOutputEvent(event Event) error { /* ... */ }
```

### Reducing Cognitive Complexity
**Problem**: Deeply nested conditionals

**Solution**: Early returns and guard clauses
```go
// BEFORE: Deep nesting
func process(data *Data) error {
    if data != nil {
        if data.IsValid() {
            if data.Type == "special" {
                // process special
            } else {
                // process normal
            }
        }
    }
}

// AFTER: Guard clauses
func process(data *Data) error {
    if data == nil {
        return errors.New("data is nil")
    }

    if !data.IsValid() {
        return errors.New("invalid data")
    }

    if data.Type == "special" {
        return processSpecial(data)
    }

    return processNormal(data)
}
```

### File Organization
**Problem**: 1000+ line file with multiple responsibilities

**Solution**: Split into focused files
```
// BEFORE:
pkg/processor.go (1000 lines)

// AFTER:
pkg/
  processor.go         (interface definition, 50 lines)
  event_processor.go   (event handling, 200 lines)
  output_processor.go  (output handling, 150 lines)
  state_manager.go     (state management, 180 lines)
  processor_test.go    (tests for processor)
  event_processor_test.go
  output_processor_test.go
  state_manager_test.go
```

## Common Pitfalls to Avoid

### 1. Superficial Refactoring
❌ **Wrong**: Moving code without decomposing
```go
// Just moved from file A to file B, still 200 lines
func giantFunction() { /* same complex code */ }
```

✅ **Right**: Actually decompose and simplify
```go
// Broken into logical pieces
func orchestrator() {
    step1()
    step2()
    step3()
}
```

### 2. Creating Temporary Files
❌ **Wrong**: `event_processor_refactored.go`
✅ **Right**: Refactor in place or create proper new structure

### 3. Ignoring Compilation
❌ **Wrong**: Making 10 changes then trying to compile
✅ **Right**: Compile after each change

### 4. Breaking Tests
❌ **Wrong**: Refactor first, fix tests later
✅ **Right**: Keep tests passing throughout refactoring

### 5. Mixing Concerns
❌ **Wrong**: Refactoring + bug fixes + new features in one commit
✅ **Right**: Separate commits for each type of change

## Package Structure Best Practices

### Standard Go Layout
```
project/
├── cmd/               # CLI commands (minimal logic)
│   └── app/
│       └── main.go
├── pkg/               # Public, importable packages
│   ├── processor/
│   ├── validator/
│   └── store/
├── internal/          # Private packages
│   ├── config/
│   └── utils/
└── tests/            # Integration tests
    └── fixtures/
```

### Interface-Driven Design
```go
// Define interface in consumer package
type Store interface {
    Get(key string) (string, error)
    Set(key, value string) error
}

// Implement in separate files
type RedisStore struct { /* ... */ }    // redis_store.go
type MemoryStore struct { /* ... */ }   // memory_store.go

// Generate mocks for testing
//go:generate mockgen -source=store.go -destination=mock_store.go
```

## Testing During Refactoring

### Maintain Test Coverage
```bash
# Before refactoring
go test -cover ./... | grep coverage
# coverage: 85.3% of statements

# After refactoring - should be same or higher
go test -cover ./... | grep coverage
# coverage: 87.1% of statements ✅
```

### Add Tests for New Files
Every new file created during refactoring needs tests:
```go
// new_processor.go
func ProcessData(data []byte) (Result, error) { /* ... */ }

// new_processor_test.go
func TestProcessData(t *testing.T) {
    tests := []struct {
        name    string
        input   []byte
        want    Result
        wantErr bool
    }{
        // test cases
    }
    // ...
}
```

## Verification Checklist

Before considering refactoring complete:

- [ ] All tests pass: `go test ./...`
- [ ] No lint errors: `golangci-lint run ./...`
- [ ] Complexity reduced: Check with `gocognit` and `cyclop`
- [ ] Functions under 60 lines
- [ ] Files under 500 lines
- [ ] Coverage maintained or improved
- [ ] No `os.Getenv()` calls (use `viper.BindEnv`)
- [ ] All comments end with periods
- [ ] Errors wrapped with static errors
- [ ] Clean git history (no WIP commits)

## Emergency Recovery

If refactoring goes wrong:

```bash
# Save any good work
git stash

# Reset to last known good state
git reset --hard HEAD

# Selectively apply good changes
git stash pop
git add -p

# Or start fresh from a clean branch
git checkout -b refactor-attempt-2
```

## References

- [Effective Go](https://golang.org/doc/effective_go)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Standard Go Project Layout](https://github.com/golang-standards/project-layout)
- [golangci-lint documentation](https://golangci-lint.run/)
