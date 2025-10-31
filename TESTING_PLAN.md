# Devcontainer Feature Testing Plan

## Goal
Achieve 80-90% test coverage for the devcontainer feature before merge.

## Current Status
- **Overall Project Coverage**: 66.38%
- **New Code Coverage**: 12.57%
- **Lines Missing Coverage**: 1,377

## Files Requiring Tests (Priority Order)

### Priority 1: Core Business Logic (High Value, Easier to Test)
1. ✅ `pkg/devcontainer/naming.go` - Already has good test coverage
2. ✅ `pkg/devcontainer/validation.go` - Already has good test coverage
3. ✅ `pkg/devcontainer/ports.go` - Already has good test coverage
4. 🔴 `pkg/devcontainer/config_loader.go` (190 lines) - **Needs refactoring + tests**
5. 🔴 `pkg/devcontainer/runtime.go` (124 lines) - **Needs interface + mocks**

### Priority 2: Container Runtime Abstraction (Requires Mocks)
6. 🟡 `pkg/container/detector.go` (43 lines) - **Needs exec mocking strategy**
7. 🔴 `pkg/container/docker.go` (152 lines) - **Needs extensive mocking**
8. 🟡 `pkg/container/podman.go` (166 lines) - **Needs extensive mocking** (some tests exist)
9. 🔴 `pkg/container/common.go` (81 lines) - **Needs exec mocking**

### Priority 3: Command Execution Layer (Requires Integration)
10. 🔴 `internal/exec/devcontainer.go` (310 lines) - **Needs refactoring into smaller functions**
11. 🔴 `internal/exec/devcontainer_helpers.go` (82 lines) - **Needs TUI mocking**
12. 🔴 `internal/exec/devcontainer_identity.go` (82 lines) - **Needs auth mocking**

### Priority 4: CLI Layer (Integration Tests)
13. 🟡 `cmd/devcontainer/helpers.go` (28 lines) - **Needs completion tests**

## Testing Strategy

### Phase 1: Mock Infrastructure (Days 1-2)
**Goal**: Create reusable test infrastructure

1. **Create container runtime mock interface**
   - Mock for `container.Runtime` interface
   - Helper functions for common test scenarios
   - Mock exec.Command using testable wrappers

2. **Create config loader test helpers**
   - Mock filesystem operations
   - Test fixture devcontainer.json files
   - Helper to create test configs

3. **Create auth/identity mocks**
   - Mock auth manager
   - Mock identity providers
   - Test credential paths

### Phase 2: Unit Tests for Core Logic (Days 3-4)
**Goal**: Test business logic in isolation

1. **`pkg/devcontainer/config_loader.go`**
   - Refactor: Extract file reading into interface
   - Tests: Valid configs, invalid configs, missing files
   - Tests: Template variable expansion
   - Tests: Include/extend resolution

2. **`pkg/devcontainer/runtime.go`**
   - Refactor: Accept Runtime interface
   - Tests: Create, start, stop, remove workflows
   - Tests: Error handling for each operation
   - Tests: State transitions

3. **`pkg/container/detector.go`**
   - Refactor: Extract exec.LookPath and exec.Command calls
   - Tests: Env var detection
   - Tests: Docker detection
   - Tests: Podman detection
   - Tests: No runtime available

### Phase 3: Container Runtime Tests (Days 5-6)
**Goal**: Test Docker/Podman abstraction

1. **`pkg/container/docker.go`**
   - Refactor: Extract command execution
   - Tests: All CRUD operations (Create, Start, Stop, Remove)
   - Tests: Exec (interactive and non-interactive)
   - Tests: List with filters
   - Tests: Inspect
   - Tests: Error handling (container not found, etc.)

2. **`pkg/container/podman.go`**
   - Similar to docker.go
   - Tests: Podman-specific JSON parsing
   - Tests: Output cleaning

3. **`pkg/container/common.go`**
   - Tests: buildExecArgs with various options
   - Tests: Exit code propagation
   - Tests: Interactive vs non-interactive exec

### Phase 4: Execution Layer Tests (Days 7-8)
**Goal**: Test command execution logic

1. **`internal/exec/devcontainer_helpers.go`**
   - Refactor: Extract TUI operations into interface
   - Tests: runWithSpinner with success/failure
   - Tests: stopContainer, removeContainer
   - Tests: Container state checks

2. **`internal/exec/devcontainer_identity.go`**
   - Tests: addCredentialMounts with various path types
   - Tests: Path expansion (~/.aws)
   - Tests: Mount string generation
   - Tests: XDG environment setup

3. **`internal/exec/devcontainer.go`**
   - Refactor: Break 310-line file into smaller functions
   - Tests: ExecuteDevcontainerStart
   - Tests: ExecuteDevcontainerStop
   - Tests: ExecuteDevcontainerAttach
   - Tests: ExecuteDevcontainerExec
   - Tests: ExecuteDevcontainerList
   - Tests: ExecuteDevcontainerRemove

### Phase 5: CLI Tests (Day 9)
**Goal**: Test command-line interface

1. **`cmd/devcontainer/helpers.go`**
   - Tests: devcontainerNameCompletion
   - Tests: getDevcontainerName with prompts
   - Tests: Runtime selection logic

2. **Integration tests**
   - CLI smoke tests for main commands
   - Test with mock runtime to avoid Docker/Podman dependency

## Refactoring Guidelines

### Make Code Testable Without Breaking Changes
1. **Extract interfaces for external dependencies**
   - FileSystem operations
   - Exec command execution
   - TUI/spinner operations

2. **Break large functions into smaller ones**
   - Max 50-60 lines per function
   - Single responsibility principle
   - Pure functions where possible

3. **Use dependency injection**
   - Accept interfaces instead of concrete types
   - Optional functional options for test hooks

4. **Maintain backward compatibility**
   - Internal refactoring only
   - No changes to public APIs
   - No changes to CLI interface

## Testing Patterns to Use

### Pattern 1: Table-Driven Tests
```go
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name     string
        input    InputType
        expected OutputType
        wantErr  bool
    }{
        // test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test implementation
        })
    }
}
```

### Pattern 2: Mock Runtime Pattern
```go
type mockRuntime struct {
    createFunc func(context.Context, *container.CreateOptions) (string, error)
    // other methods
}

func (m *mockRuntime) Create(ctx context.Context, opts *container.CreateOptions) (string, error) {
    return m.createFunc(ctx, opts)
}
```

### Pattern 3: Test Fixtures
```go
// testdata/devcontainer.json - valid config
// testdata/invalid.json - invalid config
// testdata/with-extends.json - config with extends

func loadTestFixture(t *testing.T, name string) []byte {
    data, err := os.ReadFile(filepath.Join("testdata", name))
    require.NoError(t, err)
    return data
}
```

## Success Criteria

- [ ] All files have > 80% line coverage
- [ ] All critical paths have tests
- [ ] All error paths have tests
- [ ] Tests are fast (no real Docker/Podman calls)
- [ ] Tests are deterministic (no flaky tests)
- [ ] Tests follow project conventions
- [ ] Mock infrastructure is reusable
- [ ] Documentation for running tests

## Estimated Effort

- **Mock Infrastructure**: ~500 lines
- **Unit Tests**: ~2,000-3,000 lines
- **Refactoring**: ~300 lines modified
- **Total**: ~3,000 lines of test code
- **Timeline**: Iterative approach, commit after each milestone

## Pragmatic Phased Approach

Given the scale (2000 lines of untested code), we'll use an iterative commit strategy:

### Iteration 1: Foundation (Target: 30-40% coverage) ✅ COMPLETE
- ✅ Detector tests - pkg/container/detector_test.go
- ✅ Config deserialization tests - pkg/devcontainer/config_loader_test.go (818 lines)
- ✅ Naming/validation tests (already done)
- **Coverage achieved: 48.9%** → 71.5% with runtime tests
- **Commit checkpoint**

### Iteration 2: Runtime Layer (Target: 70-80% coverage) ✅ COMPLETE
- ✅ Runtime.go tests - pkg/devcontainer/runtime_test.go (100% coverage for runtime.go)
- ✅ Common.go tests - pkg/container/common_test.go (100% coverage for helper functions)
  - buildCreateArgs with 8 comprehensive scenarios
  - All helper functions (addRuntimeFlags, addMetadata, addResourceBindings, addImageAndCommand)
  - buildExecArgs with 7 scenarios
  - Note: execWithRuntime not tested (requires exec.Command mocking, low value for coverage boost)
- ✅ Docker/Podman tests - pkg/container/docker_test.go and podman_test.go
  - getString and parseLabels helper functions (docker)
  - extractPodmanName, parseLabelsMap, parsePodmanContainer, parsePodmanContainers (podman)
  - NewDockerRuntime, NewPodmanRuntime constructor tests
  - Info() method tests (skip if runtime not available)
  - Inspect() method test for podman
  - Attach() behavioral tests with nolint:dupl justification (both Docker and Podman)
  - List() integration tests (skip if runtime not available, accept nil as valid for empty results)
- **Coverage achieved: 71.5% (devcontainer pkg), 37.8% (container pkg - up from 19.9%)**
- **Rationale for container pkg at 37.8%**: ~60% of code wraps exec.Command calls (inherently hard to test). All testable pure functions have 100% coverage. Integration tests provided for Docker/Podman when available.
- **Commit checkpoint**

### Iteration 3: Execution Layer (Target: 80-90% coverage) 🔄 IN PROGRESS
- ✅ Devcontainer helpers tests - internal/exec/devcontainer_helpers_test.go
  - ✅ TestIsContainerRunning - comprehensive status checking (12 scenarios) with nolint:dupl justification
  - Note: Other helpers require mocking Runtime interface (complex, may need interface generation)
- ⏳ Identity/credential tests - TODO
- ⏳ Core exec functions - TODO
- **Current progress: Foundation test for isContainerRunning helper, CI fixes for integration tests**
- **Latest fixes**:
  - Fixed Docker/Podman List integration tests to accept nil as valid for empty container lists
  - Fixed mockIdentityForShellEnv missing Paths() method after main branch merge
- **Commit checkpoint (phase 1)**

### Iteration 4: Polish (Target: 80-90% coverage)
- Edge cases
- Error paths
- CLI tests
- **Final commit**

This approach allows us to:
1. Show progress incrementally
2. Get feedback earlier
3. Adjust strategy based on learnings
4. Avoid massive single commits

## Next Steps

1. **Current**: Iteration 1 - Foundation tests
2. Run coverage after each file
3. Commit when reaching milestone targets
4. Continue until 80-90% achieved
