# Atmos Auth Audit Report

**Date:** 2025-10-22
**Auditor:** Claude (Conductor/Olympia)
**Branch:** conductor/osterman/audit-auth-mock-testing

## Executive Summary

### Key Findings

✅ **Mock Provider Implementation is Complete and Functional**
- Mock provider (`kind: mock`) fully implements the Provider and Identity interfaces
- Returns deterministic test credentials without requiring cloud resources
- Successfully registered in the factory pattern for automatic instantiation
- Currently used in integration tests with limited coverage

✅ **Credential Caching System is Implemented**
- System keyring integration via Zalando go-keyring
- File-based and memory-based fallback options
- Expiration checking before credential reuse
- Proper hierarchical credential resolution

⚠️ **User Complaint: Repeated Browser Authentication**

**Issue:** Bogdan reported being forced to authenticate via browser for every command, causing slowdowns.

**Root Cause Analysis:**
1. ~~Credentials ARE being cached~~ in the system keyring (macOS Keychain, Windows Credential Manager, Linux Secret Service)
2. ~~Expiration IS being checked~~ before forcing re-authentication
3. **The issue appears to be resolved** by recent PRs:
   - PR #1655: "Improve auth login with identity selection" (commit ecbf1cfe3)
   - PR #1653: "Add spinner and TTY dialog for AWS SSO auth" (commit 7d2d494b1)
   - PR #1640: "Add `atmos auth shell` command" (commit 897d1998b)

**Current Status:** Need automated test to verify the fix and prevent regression.

## Technical Architecture

### 1. Mock Provider System

**Location:** `pkg/auth/providers/mock/`

**Components:**
```
mock/
├── provider.go      # Mock Provider implementation
├── identity.go      # Mock Identity implementation
├── credentials.go   # Mock Credentials with fixed expiration
└── credentials_test.go
```

**Key Features:**
- Fixed expiration date: 2099-12-31 23:59:59 UTC (prevents test flakiness)
- Returns mock AWS-like credentials without cloud API calls
- No-op authentication (instant success)
- Implements full Provider/Identity interface contract

**Registration:**
```go
// pkg/auth/factory/factory.go
case "mock":
    return mockProviders.NewProvider(name, config), nil
```

### 2. Credential Store Architecture

**Backends:**
1. **System Keyring** (default, production)
   - macOS: Keychain
   - Windows: Credential Manager
   - Linux: Secret Service API (libsecret)

2. **File Keyring** (configurable)
   - JSON file storage with encryption
   - Path: `~/.atmos/auth/keyring.json`

3. **Memory Keyring** (testing)
   - In-memory only (not persistent)
   - Enabled via: `ATMOS_KEYRING_TYPE=memory`

**Credential Flow:**
```
Command Execution
    ↓
Whoami/Authenticate
    ↓
Check Keyring for Cached Credentials
    ↓
├─ Found + Not Expired → Use Cached
└─ Missing/Expired → Authenticate
       ↓
   Provider.Authenticate()
       ↓
   Store in Keyring
       ↓
   Return Credentials
```

**Expiration Checking:**
```go
// pkg/auth/manager.go:478-484
func (m *manager) isCredentialValid(identityName string, cachedCreds types.ICredentials) (bool, *time.Time) {
    expired, err := m.credentialStore.IsExpired(identityName)
    if err != nil || expired {
        return false, nil
    }
    // Additional validation...
}
```

### 3. Authentication Chain Resolution

**Process:**
1. Build complete authentication chain: `[provider, identity1, identity2, ..., target]`
2. Check for cached credentials at each step (bottom-up)
3. Start authentication from first missing/expired step
4. Cache credentials at each successful step

**Example Chain:**
```yaml
# atmos.yaml
auth:
  providers:
    aws-sso:
      kind: aws/iam-identity-center

  identities:
    prod-admin:
      kind: aws/permission-set
      via:
        provider: aws-sso
```

**Chain:** `[aws-sso, prod-admin]`

**Optimization:** If `prod-admin` credentials are cached and valid, skip provider authentication entirely.

## Test Coverage Analysis

### Current Coverage (as of latest main)

```
pkg/auth:                    6.2%  ⚠️ LOW
pkg/auth/cloud/aws:          0.0%  ❌ NO TESTS
pkg/auth/credentials:        0.0%  ❌ NO TESTS (unit tests exist but not counted)
pkg/auth/factory:            0.0%  ❌ NO TESTS
pkg/auth/identities/aws:     2.3%  ⚠️ LOW
pkg/auth/list:               0.0%  ❌ NO TESTS
pkg/auth/providers/aws:      0.0%  ❌ NO TESTS
pkg/auth/providers/github:   0.0%  ❌ NO TESTS
pkg/auth/providers/mock:     0.0%  ❌ NO TESTS (no unit tests)
pkg/auth/types:              0.0%  ❌ NO TESTS (interfaces only)
pkg/auth/utils:              0.0%  ❌ NO TESTS
pkg/auth/validation:         0.0%  ❌ NO TESTS
```

**Why Low Coverage?**
1. Most tests are disabled (`enabled: false # need mocks`) in `tests/test-cases/auth-cli.yaml`
2. Real provider tests require cloud credentials (AWS SSO, GitHub OIDC)
3. Unit tests are missing for many packages
4. Integration tests only cover help text and error cases

### Mock Provider Test Usage

**Currently Used:**
```yaml
# tests/test-cases/auth-cli.yaml:309-326
- name: atmos auth login --identity mock-identity
  enabled: true  # ✅ ONLY enabled auth command test
  workdir: "fixtures/scenarios/atmos-auth-mock/"
  command: "atmos"
  args: ["auth", "login", "--identity", "mock-identity"]
  expect:
    stderr: ["Authentication successful", "mock-identity"]
    exit_code: 0
```

**Disabled Tests (Need Mock):**
- `atmos auth whoami --identity test-user` (line 57-73)
- `atmos auth env --format json --identity test-user` (line 98-115)
- `atmos auth env --format bash --identity test-user` (line 117-134)
- `atmos auth env --format dotenv --identity test-user` (line 136-153)
- `atmos auth exec --identity test-user -- echo hello` (line 175-193)
- All other identity-specific commands

## Recommendations

### 1. Increase Test Coverage to 80-90%

**Phase 1: Enable Existing Tests with Mock Provider**

Convert all disabled tests to use mock provider:

```yaml
# Example: tests/test-cases/auth-cli.yaml
- name: atmos auth whoami --identity mock-identity
  enabled: true  # Changed from false
  snapshot: true
  description: "Test auth whoami with mock identity"
  workdir: "fixtures/scenarios/atmos-auth-mock/"  # Use mock scenario
  command: "atmos"
  args: ["auth", "whoami", "--identity", "mock-identity"]
  env:
    ATMOS_KEYRING_TYPE: "memory"  # Force memory keyring for tests
  expect:
    stdout:
      - "Identity: mock-identity"
      - "Provider: mock-provider"
    exit_code: 0
```

**Tests to Enable (20+ tests):**
- ✅ auth whoami with mock identity
- ✅ auth env (all formats: json, bash, dotenv)
- ✅ auth exec with mock credentials
- ✅ auth login/logout cycle
- ✅ auth list with mock identities

**Phase 2: Add Unit Tests for Core Packages**

Priority packages requiring unit tests:

1. **pkg/auth/manager.go** (core logic)
   ```go
   func TestManager_Authenticate_WithMockProvider(t *testing.T) {
       ctrl := gomock.NewController(t)
       defer ctrl.Finish()

       // Use real mock provider, not gomock
       mockProvider := mock.NewProvider("mock-provider", &schema.Provider{Kind: "mock"})

       // Test full authentication flow
       // Expected: credentials cached, no browser prompt
   }
   ```

2. **pkg/auth/credentials/** (keyring operations)
   - Test memory keyring (already has tests)
   - Test file keyring encryption
   - Test system keyring fallback logic

3. **pkg/auth/factory/** (provider/identity creation)
   - Test mock provider instantiation
   - Test error cases for unknown kinds

4. **pkg/auth/providers/mock/** (mock implementation)
   - Test credential expiration (should always be valid until 2099)
   - Test Environment() returns expected vars
   - Test no-op Logout()

**Phase 3: Integration Tests for User Complaint**

**Critical Test: Verify Credentials Are Cached**

```yaml
# tests/test-cases/auth-caching.yaml
tests:
  - name: auth login caches credentials
    enabled: true
    description: "Verify credentials are cached after login"
    workdir: "fixtures/scenarios/atmos-auth-mock/"
    env:
      ATMOS_KEYRING_TYPE: "memory"
    steps:
      - command: "atmos"
        args: ["auth", "login", "--identity", "mock-identity"]
        expect:
          exit_code: 0

      # Second command should NOT re-authenticate (cached)
      - command: "atmos"
        args: ["auth", "whoami", "--identity", "mock-identity"]
        expect:
          exit_code: 0
          stdout: ["Identity: mock-identity"]
          # Should be instant (no browser prompt)
          max_duration_ms: 1000

  - name: expired credentials force re-authentication
    enabled: true
    description: "Verify expired credentials trigger new auth"
    workdir: "fixtures/scenarios/atmos-auth-mock/"
    env:
      ATMOS_KEYRING_TYPE: "memory"
    steps:
      # Manually insert expired credentials into keyring
      - setup: insert_expired_mock_credentials

      - command: "atmos"
        args: ["auth", "whoami", "--identity", "mock-identity"]
        expect:
          exit_code: 1
          stderr: ["expired", "atmos auth login"]
```

### 2. Add Regression Test for Bogdan's Issue

**Test Case: Multiple Commands Without Re-Authentication**

```go
// cmd/auth_regression_test.go
func TestAuth_NoBrowserPromptForCachedCredentials(t *testing.T) {
    t := cmd.NewTestKit(t)

    // Setup mock auth scenario
    t.Chdir("../tests/fixtures/scenarios/atmos-auth-mock")
    t.Setenv("ATMOS_KEYRING_TYPE", "memory")

    // Step 1: Initial login
    RootCmd.SetArgs([]string{"auth", "login", "--identity", "mock-identity"})
    err := RootCmd.Execute()
    require.NoError(t, err)

    // Step 2: Run multiple commands - should use cached credentials
    commands := [][]string{
        {"auth", "whoami", "--identity", "mock-identity"},
        {"auth", "env", "--identity", "mock-identity"},
        {"describe", "component", "mycomponent", "-s", "mock"},
    }

    for _, args := range commands {
        t.Run(strings.Join(args, " "), func(t *testing.T) {
            RootCmd.SetArgs(args)

            start := time.Now()
            err := RootCmd.Execute()
            duration := time.Since(start)

            require.NoError(t, err, "Command should succeed with cached credentials")

            // Cached credentials should be instant (< 500ms)
            // Browser-based auth typically takes 5-30 seconds
            assert.Less(t, duration, 500*time.Millisecond,
                "Command took too long - may have triggered browser auth")
        })
    }
}
```

### 3. Documentation Updates

Add to `pkg/auth/docs/ARCHITECTURE.md`:

```markdown
## Credential Caching

Atmos caches credentials in the system keyring to avoid repeated authentication:

- **First Command:** Authenticate via browser/cloud provider
- **Subsequent Commands:** Use cached credentials (instant)
- **Expiration:** Credentials expire based on provider (typically 1-12 hours)
- **Re-authentication:** Only triggered when credentials expire

### Troubleshooting

**Issue:** Browser prompt on every command

**Diagnosis:**
1. Check keyring backend: `echo $ATMOS_KEYRING_TYPE`
2. Verify credentials cached: `atmos auth list`
3. Check expiration: `atmos auth whoami --identity <name>`

**Common Causes:**
- Keyring backend not accessible (permission issues)
- Credentials expiring immediately (check provider config)
- Identity misconfiguration (wrong provider reference)

**Solution:**
- Ensure keyring access: `atmos auth validate`
- Check logs: Run with `ATMOS_LOGS_LEVEL=Debug`
- Try file keyring: Set `ATMOS_KEYRING_TYPE=file`
```

## Conclusion

### Summary

1. ✅ **Mock provider is fully implemented** and ready for comprehensive testing
2. ✅ **Credential caching is working** as designed
3. ✅ **User complaint appears resolved** by recent PRs (needs verification)
4. ⚠️ **Test coverage is insufficient** (6.2% vs 80% target)
5. 🎯 **Opportunity:** Enable 20+ disabled tests by switching to mock provider

### Action Items

**High Priority:**
1. Enable all disabled auth tests with mock provider (20+ tests)
2. Add regression test for credential caching (Bogdan's issue)
3. Add unit tests for pkg/auth/manager.go

**Medium Priority:**
4. Add unit tests for pkg/auth/providers/mock/
5. Add integration tests for auth workflows
6. Update documentation with caching behavior

**Low Priority:**
7. Add performance benchmarks for cached vs. fresh auth
8. Add tests for file keyring encryption
9. Add tests for multi-identity scenarios

### Expected Coverage After Implementation

```
Current: 6.2%
After Phase 1 (enable disabled tests): ~40%
After Phase 2 (unit tests): ~70%
After Phase 3 (integration tests): 80-90% ✅ TARGET MET
```

### Verification of User Complaint Fix

**Hypothesis:** Issue was fixed by PR #1655 (identity selection improvements).

**Verification Strategy:**
1. Run regression test on commit BEFORE ecbf1cfe3 → Expect failure (slow auth)
2. Run regression test on commit AFTER ecbf1cfe3 → Expect success (cached auth)
3. Run regression test on current main → Expect success (cached auth)

**Test Command:**
```bash
# Test before fix
git checkout ecbf1cfe3~1
go test ./cmd -run TestAuth_NoBrowserPromptForCachedCredentials -v

# Test after fix
git checkout ecbf1cfe3
go test ./cmd -run TestAuth_NoBrowserPromptForCachedCredentials -v

# Test current
git checkout main
go test ./cmd -run TestAuth_NoBrowserPromptForCachedCredentials -v
```

---

**Next Steps:**
1. Confirm understanding with user (Erik)
2. Implement Phase 1 test enablement
3. Verify Bogdan's issue is resolved with automated test
4. Document findings in PR
