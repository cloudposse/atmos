# PRD: Credential Retrieval Consolidation

> **Status: ✅ IMPLEMENTED**
> Implemented in commits [76b0d1d25](https://github.com/cloudposse/atmos/commit/76b0d1d25) (consolidate logic) and [5f1e01d33](https://github.com/cloudposse/atmos/commit/5f1e01d33) (address feedback).
> Implementation: `pkg/auth/manager.go:loadCredentialsWithFallback()`

## Implementation Summary

We successfully implemented **Option 1: Extract Common Retrieval Logic** as recommended in this PRD.

### What Was Implemented

**Created `loadCredentialsWithFallback()` method** (`pkg/auth/manager.go`):
```go
func (m *manager) loadCredentialsWithFallback(ctx context.Context, identityName string) (types.ICredentials, error)
```

This single method now handles all credential retrieval with consistent fallback logic:
1. **Fast path:** Try keyring cache first
2. **Slow path:** Fall back to identity storage (AWS files, etc.) if keyring returns `ErrCredentialsNotFound`
3. **Error handling:** Properly propagates errors vs. "not found" conditions

### Refactored Call Sites

All three problematic code paths now use `loadCredentialsWithFallback()`:

1. ✅ **`GetCachedCredentials`** - Used by `atmos auth whoami`, `atmos auth shell`
2. ✅ **`findFirstValidCachedCredentials`** - Used during auth chain optimization
3. ✅ **`getChainCredentials`** (formerly `retrieveCachedCredentials`) - Used by terraform execution

### Test Coverage

Added comprehensive regression tests:
- `TestManager_retrieveCachedCredentials_TerraformFlow_Regression` - Reproduces original bug
- `TestRetrieveCachedCredentials_KeyringMiss_IdentityStorageFallback` - Verifies fallback behavior
- Integration tests covering all three code paths with file-based credentials

### Result

✅ **All auth commands now work consistently** whether credentials are in keyring or identity storage
✅ **Terraform commands fixed** - No longer fail with file-based credentials
✅ **Single source of truth** - All credential retrieval goes through one method
✅ **Tested** - Regression tests prevent future divergence

## Problem Statement (Historical)

We **had** three different code paths that retrieved credentials with **inconsistent fallback logic**, leading to bugs where some commands worked (e.g., `atmos auth whoami`) while others failed (e.g., `atmos terraform plan`) with the exact same credentials.

## Original Architecture Analysis (Before Fix)

### Three Credential Retrieval Code Paths (Historical)

#### 1. `GetCachedCredentials` (Used by `atmos auth whoami`, `atmos auth shell`, etc.)
**Location:** `pkg/auth/manager.go:195-249`

```go
func (m *manager) GetCachedCredentials(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
    // 1. Try keyring
    creds, err := m.credentialStore.Retrieve(identityName)

    if err != nil {
        // 2. Fall back to identity storage (AWS files)
        if errors.Is(err, credentials.ErrCredentialsNotFound) {
            info := m.buildWhoamiInfoFromEnvironment(identityName)
            if info.Credentials != nil {
                // SUCCESS: Returns credentials from files
            }
        }
    }
}
```

**Fallback behavior:** ✅ Keyring → Identity Storage (AWS files)

#### 2. `findFirstValidCachedCredentials` (Used during initial authentication check)
**Location:** `pkg/auth/manager.go:525-604`

```go
func (m *manager) findFirstValidCachedCredentials() int {
    for i := len(m.chain) - 1; i >= 0; i-- {
        // 1. Try keyring
        cachedCreds, err := m.credentialStore.Retrieve(identityName)

        if err != nil {
            // 2. Fall back to identity storage (AWS files)
            if !errors.Is(err, credentials.ErrCredentialsNotFound) {
                continue
            }
            identity, exists := m.identities[identityName]
            if exists {
                loadedCreds, loadErr := identity.LoadCredentials(context.Background())
                // SUCCESS: Uses credentials from files
            }
        }
    }
}
```

**Fallback behavior:** ✅ Keyring → Identity Storage (AWS files)

#### 3. `retrieveCachedCredentials` (Used by terraform execution via `fetchCachedCredentials`)
**Location:** `pkg/auth/manager.go:703-737` (BEFORE our fix)

```go
func (m *manager) retrieveCachedCredentials(chain []string, startIndex int) (types.ICredentials, error) {
    identityName := chain[startIndex]
    // ONLY tries keyring
    currentCreds, err := m.credentialStore.Retrieve(identityName)
    if err != nil {
        return nil, err  // ❌ FAILS immediately, no fallback to identity storage
    }
    return currentCreds, nil
}
```

**Fallback behavior:** ❌ Keyring only (NO fallback to identity storage)

### Why Different Code Paths Exist

1. **`GetCachedCredentials`** - Public API method for retrieving credentials by name (used by CLI commands)
2. **`findFirstValidCachedCredentials`** - Determines which step in the auth chain has valid cached credentials (for optimization)
3. **`retrieveCachedCredentials`** - Retrieves credentials at a specific chain index to resume authentication (used internally during terraform exec)

The problem: **architectural duplication** without a shared abstraction for "retrieve credential from any available source."

## Why This Manifested in Terraform Commands

### Call Flow Analysis

#### Working Path (`atmos auth whoami`):
```text
cmd/auth_whoami.go
  → pkg/auth/hooks.go: AuthWhoami()
    → pkg/auth/manager.go: Whoami()
      → pkg/auth/manager.go: GetCachedCredentials()  ✅ Has fallback
```

#### Broken Path (`atmos terraform plan`):
```text
cmd/terraform.go
  → internal/exec/terraform.go: ExecuteTerraform()
    → pkg/auth/hooks.go: TerraformPreHook()
      → pkg/auth/manager.go: Authenticate()
        → pkg/auth/manager.go: authenticateHierarchical()
          → pkg/auth/manager.go: authenticateFromIndex()
            → pkg/auth/manager.go: authenticateProviderChain()
              → pkg/auth/manager.go: fetchCachedCredentials()
                → pkg/auth/manager.go: retrieveCachedCredentials() ❌ No fallback
```

The terraform path goes **7 levels deep** through the authentication chain logic, eventually hitting the one method that doesn't have the fallback.

## Root Causes

### 1. **Code Duplication**
Three separate implementations of "retrieve credential" logic with inconsistent behavior.

### 2. **No Shared Abstraction**
No single method/interface that encapsulates "retrieve credential from any available source with proper fallback."

### 3. **Implicit Assumptions**
`retrieveCachedCredentials` implicitly assumed credentials would already be in the keyring (from prior authentication), but in reality:
- Users authenticate externally (AWS SSO via browser)
- Credentials are written to AWS credential files
- Keyring may be unavailable (Docker containers, headless systems, no D-Bus)
- We use "noop" keyring in these environments

### 4. **Testing Gap**
Tests focused on individual methods in isolation, not the full end-to-end flow from terraform command through authentication.

## Solution: Architectural Improvements

### Option 1: Extract Common Retrieval Logic (Recommended)

Create a single source of truth for credential retrieval:

```go
// loadCredentialsWithFallback retrieves credentials from keyring first,
// then falls back to identity storage (XDG directories, etc.).
// This is the ONLY method that should interact with credential storage.
func (m *manager) loadCredentialsWithFallback(ctx context.Context, identityName string) (types.ICredentials, error) {
    // 1. Try keyring
    creds, err := m.credentialStore.Retrieve(identityName)
    if err == nil {
        return creds, nil
    }

    // 2. Fall back to identity storage if keyring miss
    if !errors.Is(err, credentials.ErrCredentialsNotFound) {
        return nil, err // Real error, not just "not found"
    }

    identity, exists := m.identities[identityName]
    if !exists {
        return nil, fmt.Errorf("identity %q not found", identityName)
    }

    log.Debug("Credentials not in keyring, trying identity storage", "identity", identityName)
    loadedCreds, loadErr := identity.LoadCredentials(ctx)
    if loadErr != nil {
        return nil, fmt.Errorf("failed to load credentials from identity storage: %w", loadErr)
    }
    if loadedCreds == nil {
        return nil, fmt.Errorf("loaded credentials are nil for identity %q", identityName)
    }

    log.Debug("Loaded credentials from identity storage", "identity", identityName)
    return loadedCreds, nil
}
```

Then refactor all three paths to use this single method:

1. `GetCachedCredentials` → calls `loadCredentialsWithFallback`
2. `findFirstValidCachedCredentials` → calls `loadCredentialsWithFallback`
3. `getChainCredentials` (formerly `retrieveCachedCredentials`) → calls `loadCredentialsWithFallback`

**Benefits:**
- ✅ Single source of truth
- ✅ Consistent behavior across all commands
- ✅ Easy to test
- ✅ Easy to maintain
- ✅ Future changes only need to be made in one place

**Tradeoffs:**
- Requires refactoring three call sites
- Need to ensure context is properly passed through

### Option 2: Interface-Based Abstraction

Define a `CredentialRetriever` interface:

```go
type CredentialRetriever interface {
    Retrieve(ctx context.Context, identityName string) (types.ICredentials, error)
}

type FallbackRetriever struct {
    keyring    types.CredentialStore
    identities map[string]types.Identity
}

func (r *FallbackRetriever) Retrieve(ctx context.Context, identityName string) (types.ICredentials, error) {
    // Same logic as retrieveCredentialWithFallback
}
```

**Benefits:**
- ✅ Clean separation of concerns
- ✅ Easy to test with mocks
- ✅ Can swap implementations

**Tradeoffs:**
- More complex architecture
- Requires more extensive refactoring

### Option 3: Credential Store Enhancement (Alternative)

Make the credential store itself handle the fallback:

```go
type FallbackCredentialStore struct {
    keyring    types.CredentialStore
    identities map[string]types.Identity
}

func (s *FallbackCredentialStore) Retrieve(alias string) (types.ICredentials, error) {
    // Try keyring first, fall back to identity storage
}
```

**Benefits:**
- ✅ Centralized at the storage layer
- ✅ Transparent to callers

**Tradeoffs:**
- Violates single responsibility principle (store shouldn't know about identities)
- Credential store becomes tightly coupled to identity implementation

## Recommended Approach

**Option 1: Extract Common Retrieval Logic**

1. Create `retrieveCredentialWithFallback` as the single retrieval method
2. Refactor all three code paths to use it
3. Add comprehensive integration tests (see below)

## Testing Strategy to Prevent This Class of Bug

### Current Testing Gap

Our tests verified individual methods but missed the integration:
- ✅ `GetCachedCredentials` tested with fallback
- ✅ `findFirstValidCachedCredentials` tested with fallback
- ❌ `retrieveCachedCredentials` NOT tested with fallback
- ❌ Full terraform execution flow NOT tested with file-based credentials

### Required Tests

#### 1. Reproduce Original Bug (Regression Test)

Before applying the fix, write a test that fails:

```go
func TestTerraformAuthFlow_WithFileBasedCredentials(t *testing.T) {
    // Setup: Credentials in identity storage, NOT in keyring
    store := &keyringMissStore{} // Always returns ErrCredentialsNotFound

    identity := &mockIdentityWithStorage{
        creds: &mockCreds{expired: false},
    }

    m := &manager{
        identities: map[string]types.Identity{
            "test-identity": identity,
        },
        credentialStore: store,
        chain: []string{"test-provider", "test-identity"},
    }

    // This is the EXACT code path used by terraform execution
    creds, err := m.retrieveCachedCredentials(m.chain, 1)

    // BEFORE FIX: This fails with "credentials not found"
    // AFTER FIX: This succeeds by loading from identity storage
    require.NoError(t, err)
    assert.NotNil(t, creds)
}
```

#### 2. End-to-End Integration Tests

Test the FULL command flow:

```go
func TestAuthIntegration_AllCommands_WithFileBasedCredentials(t *testing.T) {
    tests := []struct {
        name        string
        command     func(*manager) error
        description string
    }{
        {
            name: "whoami command",
            command: func(m *manager) error {
                _, err := m.Whoami(context.Background(), "test-identity")
                return err
            },
            description: "Used by: atmos auth whoami",
        },
        {
            name: "authenticate for terraform",
            command: func(m *manager) error {
                _, err := m.Authenticate(context.Background(), "test-identity")
                return err
            },
            description: "Used by: atmos terraform plan/apply/etc",
        },
        {
            name: "get cached credentials",
            command: func(m *manager) error {
                _, err := m.GetCachedCredentials(context.Background(), "test-identity")
                return err
            },
            description: "Used by: atmos auth shell, auth status",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup: File-based credentials, no keyring
            m := setupManagerWithFileBasedCredentials(t)

            // Execute: Command should work regardless of code path
            err := tt.command(m)

            // Verify: All commands work with file-based credentials
            require.NoError(t, err, "%s should work with file-based credentials", tt.description)
        })
    }
}
```

#### 3. Shared Behavior Tests

Create a test suite that verifies ALL credential retrieval code paths have consistent behavior:

```go
func TestCredentialRetrieval_ConsistentBehavior(t *testing.T) {
    scenarios := []struct {
        name           string
        setupStore     func() types.CredentialStore
        setupIdentity  func() types.Identity
        expectSuccess  bool
        expectFallback bool
    }{
        {
            name: "keyring hit - all methods succeed",
            // ...
        },
        {
            name: "keyring miss with valid identity storage - all methods fall back",
            // ...
        },
        {
            name: "keyring miss with no identity storage - all methods fail consistently",
            // ...
        },
    }

    for _, scenario := range scenarios {
        t.Run(scenario.name, func(t *testing.T) {
            // Test ALL three retrieval code paths with SAME setup
            testGetCachedCredentials(t, scenario)
            testFindFirstValidCachedCredentials(t, scenario)
            testRetrieveCachedCredentials(t, scenario)

            // Verify ALL paths behave identically
        })
    }
}
```

## Implementation Plan (COMPLETED)

### ✅ Phase 1: Add Failing Tests
1. ✅ Added `TestManager_retrieveCachedCredentials_TerraformFlow_Regression` that reproduces the bug
2. ✅ Verified it failed before the fix (commit 76b0d1d25)
3. ✅ Verified it passes after the fix

### ✅ Phase 2: Consolidate Logic
1. ✅ Extracted `loadCredentialsWithFallback` method
2. ✅ Refactored `GetCachedCredentials` to use it
3. ✅ Refactored `findFirstValidCachedCredentials` to use it
4. ✅ Refactored `retrieveCachedCredentials` (renamed to `getChainCredentials`) to use it
5. ✅ Added comprehensive integration tests

### ⏭️ Phase 3: Enhanced Testing (Future Work)
1. ⏭️ Add end-to-end CLI tests for all command flows
2. ⏭️ Add consistency tests across all retrieval paths
3. ⏭️ Add performance tests for fallback behavior

## Success Criteria

✅ All three credential retrieval code paths use the same underlying logic
✅ All commands work consistently with file-based credentials
✅ Tests catch any future divergence in behavior
✅ Clear documentation of credential resolution order
✅ Performance impact is minimal (fallback only on keyring miss)

## Future Considerations

1. **Caching:** Should we cache identity storage lookups to avoid repeated file reads?
2. **Metrics:** Should we track how often fallback is used to understand user patterns?
3. **Configuration:** Should users be able to configure retrieval order (skip keyring entirely)?
