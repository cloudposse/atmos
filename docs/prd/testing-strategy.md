# Atmos Testing Strategy: Precondition-Based Test Skipping

## Executive Summary

This document defines the testing strategy for Atmos, focusing on creating a developer-friendly testing experience through intelligent precondition checking and test skipping rather than hard failures.

## Problem Statement

Currently, Atmos tests fail when environmental preconditions aren't met (e.g., missing AWS profiles, no network access, Git repository issues). This creates friction for developers who:
- Want to run tests locally without full environment setup
- Are working on unrelated features and don't need all preconditions
- Need to distinguish between actual code failures and missing prerequisites

## Goals

1. **Developer Experience**: Tests should gracefully skip when preconditions aren't met, not fail
2. **Clear Communication**: Skip messages should clearly explain what's missing and how to fix it
3. **Flexibility**: Developers should be able to bypass precondition checks when needed
4. **Consistency**: All tests should follow the same pattern for checking preconditions

## Design Principles

### 1. Tests Always Attempt to Run
- Tests should never be disabled or excluded at compile time
- Every test should run and make its own decision about skipping
- This ensures test discovery tools can see all tests

### 2. Fail vs Skip Decision Tree
```
Is this a precondition issue?
├─ YES: Skip with informative message
│   └─ Examples: Missing AWS profile, no network, no Git remote
└─ NO: Fail the test
    └─ Examples: Assertion failures, unexpected errors, broken code
```

### 3. Skip Messages Format
Skip messages should follow this template:
```
t.Skipf("<what's missing>: <why it's needed>. <how to fix> or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true")
```

Example:
```
t.Skipf("AWS profile 'cplive-core-gbl-identity' not configured: required for S3 backend testing. Configure AWS credentials or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true")
```

## Implementation Strategy

### Phase 1: Infrastructure (Current PR)
1. Create centralized precondition checking helpers
2. Implement environment variable override mechanism
3. Document the testing strategy

### Phase 2: Migration
1. Update existing tests to use precondition checks
2. Replace error-prone failures with intelligent skips
3. Ensure consistent skip message formatting

### Phase 3: Enforcement
1. Add linting rules for test preconditions
2. Update CI to validate skip patterns
3. Add test coverage metrics that account for skips

## Precondition Categories

### 1. Cloud Provider Configuration
- **What**: AWS profiles, Azure subscriptions, GCP projects
- **Check**: Use provider SDKs to validate configuration
- **Skip When**: Profile/credentials not available
- **Example**: `RequireAWSProfile(t, "profile-name")`

### 2. Network Dependencies
- **What**: External service availability (GitHub, APIs, etc.)
- **Check**: HTTP HEAD requests with timeout
- **Skip When**: Service unreachable or rate-limited
- **Example**: `RequireGitHubAccess(t)`

### 3. Development Tools
- **What**: Git repository, Docker, Terraform, etc.
- **Check**: Validate tool availability and configuration
- **Skip When**: Tool not installed or misconfigured
- **Example**: `RequireGitRepository(t)`

### 4. Filesystem Requirements
- **What**: Specific files, directories, or permissions
- **Check**: OS-appropriate file system checks
- **Skip When**: Missing files or insufficient permissions
- **Example**: `RequireFilePath(t, "/path/to/file")`

### 5. Build Artifacts
- **What**: Compiled binaries, generated files
- **Check**: Verify existence and freshness
- **Skip When**: Artifacts missing or stale
- **Example**: Package-level `skipReason` in TestMain

### 6. OCI Registry Authentication
- **What**: Authentication for pulling OCI images from container registries
- **Check**: Presence of GitHub token (GITHUB_TOKEN or ATMOS_GITHUB_TOKEN)
- **Skip When**: Token not configured
- **Example**: Tests pulling from ghcr.io (GitHub Container Registry)

## Environment Variables

### `ATMOS_TEST_SKIP_PRECONDITION_CHECKS`
- **Values**: `true` or unset/any other value
- **Purpose**: Bypass all precondition checks
- **Use Cases**:
  - CI environments with mocked dependencies
  - Testing specific functionality without full setup
  - Debugging test failures

### Future Environment Variables
- `ATMOS_TEST_MOCK_AWS`: Use mocked AWS services
- `ATMOS_TEST_OFFLINE`: Skip all network-dependent tests
- `ATMOS_TEST_VERBOSE_SKIP`: Show detailed skip reasoning

## Developer Workflow

### Running Tests Locally

1. **First Run - See What's Needed**:
   ```bash
   go test ./...
   # See skips with clear messages about missing preconditions
   ```

2. **Setup Required Preconditions**:
   ```bash
   # Example: Configure AWS
   aws configure --profile cplive-core-gbl-identity
   
   # Example: Ensure Git remotes
   git remote add origin https://github.com/cloudposse/atmos.git
   ```

3. **Run With Preconditions Met**:
   ```bash
   go test ./...
   # Tests run with preconditions satisfied
   ```

4. **Or Bypass Checks**:
   ```bash
   export ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true
   go test ./...
   # All precondition checks bypassed
   ```

## CI/CD Considerations

### GitHub Actions
```yaml
- name: Run Tests
  env:
    # CI can set this if using mocked services
    ATMOS_TEST_SKIP_PRECONDITION_CHECKS: ${{ secrets.SKIP_PRECONDITIONS }}
  run: |
    make test
```

### Test Reporting
- Skip counts should be tracked alongside pass/fail
- Dashboards should show trends in skip rates
- High skip rates might indicate environment issues

## Success Metrics

1. **Developer Satisfaction**: Reduced frustration with test failures
2. **Test Reliability**: Clear distinction between environment and code issues
3. **Onboarding Time**: New developers can run tests immediately
4. **Test Coverage**: More developers running more tests locally

## Migration Guide

### For Existing Tests

**Before**:
```go
func TestAWSFeature(t *testing.T) {
    cfg, err := LoadAWSConfig(...)
    assert.NoError(t, err) // Fails if AWS not configured
    // ...
}
```

**After**:
```go
func TestAWSFeature(t *testing.T) {
    RequireAWSProfile(t, "profile-name") // Skips if not configured
    
    cfg, err := LoadAWSConfig(...)
    assert.NoError(t, err) // Only runs if precondition met
    // ...
}
```

### For New Tests

Always consider:
1. What external dependencies does this test have?
2. Add appropriate `Require*` calls at test start
3. Use descriptive skip messages
4. Document special requirements in test comments

## Best Practices

### DO ✅
- Check preconditions at test start
- Use helper functions for common checks
- Provide actionable skip messages
- Document unusual preconditions in comments
- Use `t.Skipf()` with formatted messages

### DON'T ❌
- Check preconditions deep in test logic
- Use generic skip messages like "precondition failed"
- Mix precondition checks with test assertions
- Use `t.Skip()` without explanation
- Assume CI and local environments are identical

## Appendix A: Helper Function Reference

| Function | Purpose | Skip Condition |
|----------|---------|----------------|
| `RequireAWSProfile(t, profile)` | Check AWS configuration | Profile not available |
| `RequireGitRepository(t)` | Check Git repo | Not in Git repo |
| `RequireGitRemoteWithValidURL(t)` | Check Git remotes | No valid remote URL |
| `RequireGitHubAccess(t)` | Check GitHub connectivity | Network/rate limit issues |
| `RequireNetworkAccess(t, url)` | Check general network | URL unreachable |
| `RequireExecutable(t, name)` | Check for executable | Not in PATH |
| `RequireEnvVar(t, name)` | Check environment variable | Not set |

## Appendix B: Common Skip Message Templates

```go
// AWS Profile
t.Skipf("AWS profile '%s' not configured: required for %s. Configure AWS credentials or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true", profileName, purpose)

// Network Access
t.Skipf("Cannot reach %s: %v. Check network connection or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true", url, err)

// Git Repository
t.Skipf("Not in a Git repository: %v. Initialize a Git repo or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true", err)

// Rate Limiting
t.Skipf("GitHub API rate limit exceeded. Resets at %s. Use authenticated requests or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true", resetTime)

// Missing Tool
t.Skipf("'%s' not found in PATH: required for %s. Install the tool or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true", tool, purpose)
```

## Version History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-01-09 | Team | Initial strategy document |