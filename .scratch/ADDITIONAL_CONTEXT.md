# Additional Context for Agent Updates

## Critical Additions from User Feedback

### 1. Test Strategy Architect - Go 1.25 Features

**Missing Context:**
- Go 1.25 specific test capabilities
- `t.Chdir()` for directory changes in tests
- `t.Setenv()` for environment variables in tests
- TestKit for Cobra command isolation (already mentioned but needs more detail)
- Sandbox implementation and proper usage
- Difference between integration tests ("smoke tests") vs pure Go tests
- Preference for pure Go tests over integration tests
- Golden snapshot critical importance, regeneration, and questioning output

**Where to Add:**
- `test-strategy-architect.md` - Major additions needed in testing patterns section

### 2. PR Review Resolver - CodeRabbit Focus

**Missing Context:**
- Specific focus on CodeRabbit AI comments
- GitHub API command to filter CodeRabbit comments:
  ```bash
  gh api --paginate \
    "repos/cloudposse/atmos/pulls/<PR_NUMBER>/comments" \
    --jq '.[] | select(.body | test("CodeRabbit|AI"; "i")) | {file: .path, comment: .body}'
  ```
- Already has CodeRabbit focus but could emphasize the filtering command

**Where to Add:**
- `pr-review-resolver.md` - Add CodeRabbit filtering command to GitHub API section

### 3. Lint Resolver - Refactoring Plan Required

**Missing Context:**
- Must propose a plan for how to refactor complexity warnings
- Should suggest breaking into smaller testable functions
- NEVER use `git commit --no-verify` (already has this, good!)
- Should collaborate with Refactoring Architect for complex issues

**Where to Add:**
- `lint-resolver.md` - Already has this but could strengthen the "propose plan first" guidance

### 4. CI/CD Debugger Concept (Fix Agent)

**New Agent Concept:**
The "fix" instructions suggest a CI/CD debugging workflow:
- Use `gh pr checks <PR_NUMBER>` to check status
- Use `gh run view <RUN_ID> --log` for logs
- Use `gh run download <RUN_ID>` for offline analysis
- Focus on failing checks
- Review unresolved PR comments
- Propose fixes with rationale

**Status:** This overlaps with existing agents:
- `pr-review-resolver` - Already handles PR comments
- Could create dedicated `ci-failure-resolver` or enhance `pr-review-resolver`

**Recommendation:** Enhance `pr-review-resolver` to include CI failure debugging

---

## Detailed Changes Needed

### Test Strategy Architect - Major Update Required

#### Add Go 1.25 Testing Features Section

```markdown
## Go 1.25 Testing Features (MANDATORY)

### t.Chdir() - Test Directory Isolation
**Use for:** Tests that need to change directories
```go
func TestLoadConfig(t *testing.T) {
    t.Chdir("testdata/fixtures")  // Go 1.25+

    // Now working directory is testdata/fixtures for this test only
    config, err := LoadConfig("config.yaml")
    // Test automatically reverts to original directory
}

// Before Go 1.25 (DON'T USE):
func TestLoadConfigOld(t *testing.T) {
    oldDir, _ := os.Getwd()
    os.Chdir("testdata/fixtures")
    defer os.Chdir(oldDir)  // Manual cleanup, error-prone
}
```

### t.Setenv() - Environment Variable Isolation
**Use for:** Tests that need environment variables
```go
func TestEnvVarHandling(t *testing.T) {
    t.Setenv("ATMOS_BASE_PATH", "/tmp/test")  // Go 1.25+

    // Environment variable is set for this test only
    // Automatically cleaned up after test
    result := getBasePath()
    assert.Equal(t, "/tmp/test", result)
}
```

**Why These Are Critical:**
- Automatic cleanup (no defer needed)
- Test isolation (parallel test safety)
- Cleaner, less error-prone code
- **ALWAYS prefer t.Chdir() and t.Setenv() over manual management**
```

#### Add TestKit Deep Dive Section

```markdown
## TestKit for Cobra Command Isolation (MANDATORY)

### What is TestKit?
TestKit (`cmd.NewTestKit(t)`) provides automatic cleanup of Cobra `RootCmd` state between tests:
- Resets all flags to default values
- Clears command arguments
- Resets command state
- Prevents test pollution

### When to Use TestKit
**ALWAYS use for tests that:**
- Execute Cobra commands
- Test CLI flag parsing
- Test command behavior
- Touch `RootCmd` in any way

### TestKit Pattern
```go
func TestAtmosCommand(t *testing.T) {
    // MANDATORY: Create TestKit first
    kit := cmd.NewTestKit(t)

    // TestKit provides:
    // - kit.RootCmd: Clean RootCmd instance
    // - Automatic cleanup after test
    // - Proper test isolation

    // Set up test
    kit.RootCmd.SetArgs([]string{"terraform", "plan", "-s", "prod"})

    // Execute command
    err := kit.RootCmd.Execute()

    // Assertions
    assert.NoError(t, err)
}
```

### Without TestKit (WRONG)
```go
// ❌ WRONG: Direct RootCmd access causes test pollution
func TestCommandWrong(t *testing.T) {
    cmd.RootCmd.SetArgs([]string{"plan"})
    cmd.RootCmd.Execute()
    // Flags and state persist to next test!
}
```
```

#### Add Sandbox Implementation Section

```markdown
## Sandbox Implementation

### What is the Sandbox?
The sandbox provides isolated filesystem environments for tests:
- Temporary directory per test
- Automatic cleanup
- Prevents test interference

### Sandbox Pattern
```go
func TestWithSandbox(t *testing.T) {
    // Create sandbox
    sandbox := test.NewSandbox(t)
    defer sandbox.Cleanup()

    // Sandbox provides:
    // - sandbox.Dir: Temporary directory path
    // - sandbox.WriteFile(path, content): Write test files
    // - sandbox.ReadFile(path): Read files
    // - Automatic cleanup on test completion

    // Write test fixture
    sandbox.WriteFile("atmos.yaml", `
base_path: .
stacks:
  base_path: stacks
`)

    // Run test in sandbox
    t.Chdir(sandbox.Dir)
    result := LoadAtmosConfig()

    // Cleanup happens automatically
}
```

### When to Use Sandbox
- Tests that create files
- Tests that modify filesystem
- Tests that need isolated directories
- Integration tests with file I/O
```

#### Add Integration vs Unit Test Guidance

```markdown
## Integration Tests vs Pure Go Tests

### Preference: Pure Go Tests (90%)
**Pure Go tests are preferred because:**
- Faster execution
- Better isolation
- Easier to debug
- More focused testing
- Better coverage reporting

```go
// PREFERRED: Pure Go test with mocks
func TestProcessStack(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockFS := mock_filesystem.NewMockFileSystem(ctrl)
    mockFS.EXPECT().ReadFile("stack.yaml").Return([]byte("..."), nil)

    result, err := ProcessStack(mockFS, "stack.yaml")
    assert.NoError(t, err)
}
```

### Integration Tests: Smoke Tests (10%)
**Integration tests ("smoke tests") are for:**
- End-to-end CLI behavior
- Cross-package integration
- Real filesystem operations
- Catch-all verification

**Location:** `tests/` directory (not co-located with code)

```go
// Integration test (use sparingly)
func TestAtmosCLISmoke(t *testing.T) {
    // Build test CLI binary
    testCLI := buildTestCLI(t)

    // Run actual CLI command
    output, err := exec.Command(testCLI, "describe", "component").CombinedOutput()

    // Verify end-to-end behavior
    assert.Contains(t, string(output), "component:")
}
```

### Decision Tree: Which Test Type?

**Use Pure Go Test When:**
- ✅ Testing business logic
- ✅ Testing single package
- ✅ Can mock dependencies
- ✅ Need fast feedback

**Use Integration Test When:**
- ⚠️ Testing CLI end-to-end
- ⚠️ Testing cross-package interaction
- ⚠️ Cannot easily mock (last resort)
- ⚠️ Verifying real file operations

**Rule of Thumb:** If you can write a pure Go test with mocks, do that instead.
```

#### Strengthen Golden Snapshot Guidance

```markdown
## Golden Snapshots (CRITICAL)

### Why Golden Snapshots Are Critical
Golden snapshots are **the primary way we test CLI output**:
- Capture exact terminal output
- Include ANSI codes, colors, formatting
- Detect unintended output changes
- Catch regressions in user-facing output

### Golden Snapshot Pattern
```go
func TestCommandOutput(t *testing.T) {
    kit := cmd.NewTestKit(t)

    // Capture output
    var buf bytes.Buffer
    kit.RootCmd.SetOut(&buf)
    kit.RootCmd.SetArgs([]string{"describe", "component", "-s", "stack"})

    err := kit.RootCmd.Execute()
    assert.NoError(t, err)

    // Compare with golden file
    goldenFile := "testdata/golden/describe-component.txt"
    if *regenerateSnapshots {
        os.WriteFile(goldenFile, buf.Bytes(), 0644)
    } else {
        expected, _ := os.ReadFile(goldenFile)
        assert.Equal(t, string(expected), buf.String())
    }
}
```

### Regenerating Snapshots
**NEVER manually edit golden snapshot files!**

```bash
# CORRECT: Use flag to regenerate
go test ./tests -run TestCommandOutput -regenerate-snapshots

# WRONG: Manually editing .txt files
vim testdata/golden/describe-component.txt  # ❌ Don't do this!
```

### Why Manual Editing Fails
- Lipgloss table padding varies by terminal width
- ANSI color codes are invisible in editors
- Trailing whitespace is significant but invisible
- Unicode character widths affect column alignment
- Different environments produce different output

### When to Question Snapshot Output
**Regenerate snapshots when:**
- ✅ You intentionally changed output format
- ✅ You added new fields to output
- ✅ You fixed a formatting bug

**Question snapshots when:**
- ⚠️ Snapshot changes unexpectedly
- ⚠️ Output looks corrupted
- ⚠️ Test passes locally but fails in CI
- ⚠️ Snapshot includes sensitive data (credentials, tokens)

**Investigate before regenerating:**
1. Review the diff: What actually changed?
2. Is this change intentional?
3. Does the new output look correct?
4. Are there any security implications?

### Snapshot Best Practices
- One golden file per test case
- Name files descriptively: `describe-component-success.txt`
- Store in `testdata/golden/` directory
- Include both success and error output snapshots
- Review snapshot diffs carefully in PRs
```

---

## Priority Updates

### 🔥 CRITICAL: Test Strategy Architect
**Priority: P0 - Update immediately**
- Add Go 1.25 features (t.Chdir, t.Setenv)
- Deep dive on TestKit
- Sandbox implementation
- Integration vs pure Go test guidance
- Strengthen golden snapshot section

### 🟡 MEDIUM: PR Review Resolver
**Priority: P1 - Update soon**
- Add CodeRabbit comment filtering command
- Already has good CodeRabbit focus, just add the specific `gh api` command

### 🟢 LOW: Lint Resolver
**Priority: P2 - Nice to have**
- Already has "propose plan first" but could strengthen
- Already mentions Refactoring Architect collaboration
- Minor enhancements only

---

## Implementation Plan

### Phase 1: Test Strategy Architect (Critical)
1. Add Go 1.25 features section
2. Expand TestKit deep dive
3. Add Sandbox implementation
4. Add integration vs unit test decision tree
5. Strengthen golden snapshot guidance with "when to question" section

### Phase 2: PR Review Resolver (Medium)
1. Add CodeRabbit filtering command
2. Add example of using the command
3. Strengthen CodeRabbit comment handling

### Phase 3: Lint Resolver (Optional)
1. Strengthen "propose refactoring plan first" guidance
2. Add explicit Refactoring Architect collaboration trigger

---

## Questions for User

1. **Sandbox Implementation:** Is there existing sandbox code we should reference? (e.g., `test.NewSandbox(t)`)

2. **TestKit Details:** Is TestKit at `cmd.NewTestKit(t)` or different package?

3. **Integration Test CLI Build:** How do you build the test CLI binary? Is there a helper function?

4. **Golden Snapshots Flag:** Is `-regenerate-snapshots` the correct flag name?

5. **Smoke Tests:** Are these in `tests/` directory? Any specific naming convention?
