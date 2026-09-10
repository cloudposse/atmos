# Test Preconditions and Intelligent Skipping

## Overview

This document defines the precondition checking and intelligent test skipping capabilities in Atmos's testing framework. This is one component of Atmos's broader testing strategy, specifically addressing how tests handle missing environmental dependencies.

## Problem Statement

Tests were failing when environmental dependencies weren't available (e.g., AWS profiles, network access, Git configuration), making it difficult to:
- Distinguish between actual code failures and missing prerequisites
- Run partial test suites during development
- Onboard new developers who haven't configured all dependencies

## Solution: Intelligent Precondition Checking

### Core Capability

Tests can detect missing preconditions and skip gracefully with informative messages rather than failing. This allows developers to run test suites even without full environment setup while maintaining clear visibility into what was skipped and why.

## Implementation

### Precondition Helper Functions

Created a centralized set of helper functions in `tests/preconditions.go` that check for specific preconditions and skip tests when not met.

**Available Helpers**:

| Function | Purpose | Skip Condition |
|----------|---------|----------------|
| `RequireAWSProfile(t, profile)` | Verify AWS profile exists | Profile not configured |
| `RequireGitRepository(t)` | Ensure running in Git repo | Not a Git repository |
| `RequireGitRemoteWithValidURL(t)` | Check Git remote configuration | No valid remote URL |
| `RequireGitHubAccess(t)` | Test GitHub API connectivity | Network issues, rate limits, or `ATMOS_TEST_OFFLINE=true` |
| `RequireNetworkAccess(t, url)` | Verify network connectivity | URL unreachable or `ATMOS_TEST_OFFLINE=true` |
| `RequireLiveGitHub(t)` | Gate a canary that must reach real github.com, not the acceptance suite's local git mirror/HTTP mock | Same as `RequireGitHubAccess` |
| `RequireLiveGitHubAuthenticated(t)` | Like `RequireLiveGitHub`, and additionally requires a real `GITHUB_TOKEN` | Same as `RequireLiveGitHub`, plus `GITHUB_TOKEN` unset |
| `RequireExecutable(t, name)` | Check executable availability | Not found in PATH |
| `RequireEnvVar(t, name)` | Ensure environment variable set | Variable not set |

**Key Features**:
- Helpers return useful data when preconditions pass (e.g., GitHub rate limit info)
- Cross-platform compatible with OS-appropriate checks
- Consistent skip message format across all helpers

### Skip Message Standards

All skip messages follow a consistent format to maximize developer understanding:

```
<what's missing>: <why needed>. <how to fix> or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true
```

**Examples**:
- `"AWS profile 'dev' not configured: required for S3 backend testing. Configure AWS credentials or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true"`
- `"GitHub API rate limit too low (5 remaining): need at least 20 requests. Wait for reset at 15:30 or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true"`

### Override Mechanism

**Environment Variable**: `ATMOS_TEST_SKIP_PRECONDITION_CHECKS`
- When set to `true`, all precondition checks are bypassed
- Useful for CI environments with mocked dependencies
- Enables focused testing without full environment setup

**Implementation**:
```go
func ShouldCheckPreconditions() bool {
    return os.Getenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS") != "true"
}
```

**Environment Variable**: `ATMOS_TEST_OFFLINE`
- When set to `true`, every test that depends on live network access skips outright
  (`RequireGitHubAccess`, `RequireNetworkAccess`, and the `live_github`/`live_github_authenticated`
  canaries gated by `RequireLiveGitHub`/`RequireLiveGitHubAuthenticated`)
- Checked **independently** of `ATMOS_TEST_SKIP_PRECONDITION_CHECKS`: that flag only bypasses the
  connectivity *probes* those helpers run before a test (so CI can rely on the local git mirror
  and HTTP mock without also requiring live GitHub reachability) -- it never means "run this
  test's live network calls anyway." `ATMOS_TEST_OFFLINE` is the switch for that.

**Implementation**:
```go
func Offline() bool {
    return os.Getenv("ATMOS_TEST_OFFLINE") == "true"
}
```

### Live GitHub Canaries

Most of the acceptance suite runs against a local git mirror (`tests/testhelpers/gitmirror`) or an
HTTP mock instead of live GitHub, for speed and determinism. A small number of canary tests
(`tests/live_github_canary_test.go`) deliberately keep exercising the real, live GitHub path --
one unauthenticated `vendor pull` of a real external repo, one authenticated variant, one
unauthenticated toolchain release-asset install, and one unauthenticated raw-content `!include`
fetch.

Two test-case preconditions identify these canaries:

- **`live_github`**: the test must reach real, unauthenticated GitHub. `GITHUB_TOKEN`,
  `ATMOS_GITHUB_TOKEN`, `ATMOS_PRO_GITHUB_TOKEN`, and `GH_TOKEN` are blanked, `GH_CONFIG_DIR`
  points at an empty temp directory (defeating the `gh auth token` CLI fallback), and the local git
  mirror's `insteadOf` redirect rules are removed so the test actually reaches github.com instead
  of being silently redirected to the mirror.
- **`live_github_authenticated`**: like `live_github`, but keeps `GITHUB_TOKEN` and the git
  extraheader token injection. Skips if `GITHUB_TOKEN` is not set.

Both gate on `RequireLiveGitHub`/`RequireLiveGitHubAuthenticated`, so they honor
`ATMOS_TEST_OFFLINE` like every other live-network test. Because they still run in the normal PR
shard matrix (no separate scheduled workflow), each canary also classifies a subprocess failure as
*transient* (DNS/connect failures, timeouts, TLS trust failures, rate limits, 5xx) and skips rather
than fails in that case -- only a genuine 401/403/404 or an actual atmos bug fails the build.

### Linting and Enforcement

**Enforced Rules**:
1. **Must use `t.Skipf()` instead of `t.Skip()`**: Ensures all skips include descriptive reasons
    - Enforced via `forbidigo` linter pattern: `\.Skip\(`
    - Message: "Use t.Skipf with a descriptive reason instead of t.Skip"

2. **Environment variable handling**:
    - Production code must use `viper.BindEnv` (enforced by `forbidigo`)
    - Test files and test helpers can use `os.Getenv`/`os.Setenv`
    - Configured via `exclude-rules` in `.golangci.yml`

### Binary Freshness Detection

For CLI tests that depend on built binaries:
- TestMain checks if binary is outdated
- Sets package-level `skipReason` if rebuild needed
- Individual tests check and skip with clear message about running `make build`

## Usage Patterns

### In Test Files

```go
func TestAWSIntegration(t *testing.T) {
    // Check precondition at test start
    tests.RequireAWSProfile(t, "dev-profile")

    // Test code only runs if precondition met
    // ...
}

func TestGitHubVendoring(t *testing.T) {
    // Check multiple preconditions
    tests.RequireGitRepository(t)
    rateLimits := tests.RequireGitHubAccess(t)

    // Can make decisions based on returned data
    if rateLimits != nil && rateLimits.Remaining < 50 {
        t.Skipf("Need at least 50 API requests, only %d remaining", rateLimits.Remaining)
    }

    // Test code
    // ...
}
```

### Developer Workflow

1. **Run tests to see requirements**:
    ```bash
    go test ./...
    # SKIP: AWS profile 'dev' not configured: required for S3 backend testing...
    ```

2. **Either configure dependencies**:
    ```bash
    aws configure --profile dev
    export GITHUB_TOKEN=ghp_...
    ```

3. **Or bypass checks**:
    ```bash
    export ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true
    go test ./...
    ```

## Benefits Achieved

1. **Improved Developer Experience**: Tests skip gracefully instead of failing mysteriously
2. **Clear Communication**: Every skip explains what's missing and how to fix it
3. **Flexibility**: Developers can bypass checks when appropriate
4. **Consistency**: Standardized patterns across the test suite
5. **Maintainability**: Centralized helpers reduce duplication

## Relationship to Overall Testing Strategy

This precondition checking system is one component of Atmos's comprehensive testing approach, which also includes:
- Unit tests with mocked dependencies
- Integration tests with real services
- Acceptance tests for end-to-end validation
- Performance benchmarks
- Fuzz testing for input validation
- CI/CD automation with GitHub Actions

The precondition system specifically enhances the integration and acceptance test layers by making them more accessible to developers working in varied environments.

## Future Enhancements

- `ATMOS_TEST_MOCK_AWS`: Automatically use mocked AWS services
- `ATMOS_TEST_VERBOSE_SKIP`: Provide detailed skip reasoning
- Integration with test coverage tools to track skip patterns
