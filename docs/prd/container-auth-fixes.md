# Container Authentication Fixes

This document memorializes two critical authentication bugs that were fixed to enable proper authentication in container environments.

## Bug #1: Authentication Chain Truncation (Primary Issue)

### Symptom
In container environments (with noop keyring), assume-role identities were receiving permission set credentials instead of properly assumed role credentials. Natively (with system keyring), authentication worked correctly.

### Root Cause
The `ensureIdentityHasManager()` function (pkg/auth/manager.go:540) was unconditionally rebuilding the authentication chain when called with intermediate identities, overwriting the original chain built for the target identity.

**Bug Flow:**
1. `Authenticate("core-root/admin")` builds 3-element chain: `[provider, permission-set, assume-role]`
2. In containers with noop keyring, all credential lookups return "not found"
3. Falls back to `loadCredentialsWithFallback("permission-set")` to check file storage
4. Calls `ensureIdentityHasManager("permission-set")` to set manager reference
5. **BUG**: Rebuilds chain for permission-set: `[provider, permission-set]`
6. **OVERWRITES `m.chain`**, truncating from 3 elements to 2 elements
7. `authenticateIdentityChain()` uses truncated chain, only authenticates through permission-set
8. `PostAuthenticate()` writes permission-set credentials to assume-role identity profile

### Why Native Worked But Container Failed

**Native (system keyring):**
- Cached credentials stored in keyring after authentication
- Next run retrieves from keyring, no fallback to file storage
- `ensureIdentityHasManager` never called during cached credential retrieval
- Chain stays intact

**Container (noop keyring):**
- Noop keyring ALWAYS returns "not found" (no persistence)
- ALWAYS falls back to file storage via `loadCredentialsWithFallback`
- ALWAYS calls `ensureIdentityHasManager` which overwrites chain
- Chain gets truncated, authentication incomplete

### Fix
Added check in `ensureIdentityHasManager()` to preserve existing authentication chains:

```go
// If chain exists but for a DIFFERENT identity, don't overwrite it!
// This happens when loading cached credentials for an intermediate identity
// (e.g., permission set) while authenticating a target identity (e.g., assume role).
// The existing chain is for the target identity and should not be replaced.
if len(m.chain) > 0 {
    // Chain exists for a different identity - just set manager reference
    // using the existing chain without rebuilding.
    return m.setIdentityManager(identityName)
}
```

**Commit:** `838d40c70` - "fix: Prevent authentication chain truncation in container environments"

### Tests
- `TestManager_fetchCachedCredentials` - Verifies `startIndex + 1` behavior for chain continuation
- Existing integration tests verify end-to-end authentication works in both native and container environments

---

## Bug #2: SSO Token Caching (Secondary Issue - Already Fixed)

### Symptom
`atmos auth login` would always prompt for device authorization even when a valid SSO session existed.

### Root Cause
AWS SSO tokens were not being cached between `atmos` invocations. Each login would start a new device authorization flow.

### Fix
Implemented XDG-compliant SSO token caching in `pkg/auth/providers/aws/sso.go`:
- Cache location: `~/.cache/atmos/aws-sso/<provider-name>/token.json`
- Validates expiration with 5-minute buffer
- Validates configuration match (start URL, region, etc.)
- Non-fatal operations (graceful degradation if cache fails)

**Commit:** Multiple commits in the PR

### Tests
- Manual testing with `atmos auth login` verifies token reuse
- Container support via mounted cache directory in `scripts/test-geodesic-prebuilt.sh`

---

## Verification

Both fixes were verified to work:
- **Natively**: Multiple assume-role identities authenticate correctly
- **In Containers**: Same identities authenticate correctly with proper credentials
- **Mixed Usage**: Identities tested natively can be used in containers and vice versa

## Related Files

### Code Changes
- `pkg/auth/manager.go` - Chain preservation fix
- `pkg/auth/manager_test.go` - Test updates
- `pkg/auth/providers/aws/sso.go` - SSO token caching
- `scripts/test-geodesic-prebuilt.sh` - Container cache mount

### Documentation
- This file (`docs/prd/container-auth-fixes.md`)
- Commit messages with full technical details
- Inline code comments explaining the fixes
