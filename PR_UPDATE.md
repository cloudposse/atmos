# PR Title

fix: Resolve authentication issues in CI and container environments

# PR Description

## Summary

This PR fixes multiple critical authentication bugs that affected CI environments and containers, while also improving the overall authentication user experience.

## What Changed

### 1. Container Authentication Chain Truncation (Primary Fix)
**Problem:** In container environments with noop keyring, assume-role identities were receiving permission set credentials instead of properly assumed role credentials.

**Root Cause:** The `ensureIdentityHasManager()` function was unconditionally rebuilding the authentication chain when loading cached credentials for intermediate identities, truncating the chain from `[provider, permission-set, assume-role]` to `[provider, permission-set]`.

**Fix:** Added check in `ensureIdentityHasManager()` to preserve existing authentication chains when they belong to a different (target) identity.

**Impact:** Authentication now works correctly in containers for all identity types, including multi-step chains.

**Commit:** 838d40c70

### 2. SSO Token Caching
**Problem:** `atmos auth login` would always prompt for device authorization even when a valid SSO session existed.

**Fix:** Implemented XDG-compliant SSO token caching at `~/.cache/atmos/aws-sso/<provider-name>/token.json` with:
- Expiration validation (5-minute buffer)
- Configuration match validation
- Non-fatal graceful degradation
- Container support via mounted cache directory

**Impact:** Users no longer need to repeatedly authorize devices when SSO sessions are still valid.

**Commit:** fbbba58d1

### 3. CI Environment Interactive Mode Detection
**Problem:** Interactive prompts would hang in CI environments, causing test failures.

**Fix:**
- Improved TTY detection to check both stdin and stdout
- Added CI environment detection (`CI=true`, `GITHUB_ACTIONS`, etc.)
- Disabled interactive prompts when stdout is piped or in CI

**Impact:** Tests run reliably in CI without hanging on prompts.

**Commits:** 03ecd7966, e771a34bb, b5cf41c32

### 4. Identity Flag Parsing Regression
**Problem:** `--identity` flag parsing was broken across terraform/helmfile/packer commands.

**Fix:** Centralized identity flag parsing logic and added comprehensive shell completion support.

**Impact:** `--identity` flag works consistently across all commands.

**Commits:** 6fb7c6d00, aa6a34766, a0cee9426

### 5. Safe Argument Parsing with Shell Quoting
**Problem:** Arguments with spaces and special characters could break when passed to spawned processes.

**Fix:** Implemented proper shell quoting using `mvdan.cc/sh/v3/syntax` for safe argument handling.

**Impact:** Arguments with spaces, quotes, and special characters are handled correctly.

**Commit:** 163d7a419

## Testing

- ✅ All auth tests pass
- ✅ Verified native authentication (system keyring)
- ✅ Verified container authentication (noop keyring)
- ✅ Verified multi-step authentication chains (permission-set → assume-role)
- ✅ CI tests pass without hanging

## Documentation

Added comprehensive PRD documentation:
- `docs/prd/container-auth-fixes.md` - Complete explanation of container authentication bugs and fixes
- Updated `docs/prd/credential-retrieval-consolidation.md` - Added container auth chain bug section

## Files Changed

### Core Authentication
- `pkg/auth/manager.go` - Chain preservation fix, credential retrieval improvements
- `pkg/auth/manager_test.go` - Test updates
- `pkg/auth/providers/aws/sso.go` - SSO token caching implementation
- `pkg/auth/identities/aws/permission_set.go` - Graceful error handling
- `pkg/auth/providers/mock/identity.go` - Fixed mock provider environment variables

### Container Support
- `scripts/test-geodesic-prebuilt.sh` - Added cache directory mount

### CI/Testing
- `tests/test-cases/auth-mock.yaml` - Fixed time-based snapshot issues
- Multiple test files for improved coverage

### Documentation
- `docs/prd/container-auth-fixes.md` - New PRD documenting authentication fixes
- `docs/prd/credential-retrieval-consolidation.md` - Updated with container insights

## Why These Changes Matter

### Container Environments
Container environments use noop keyring (no dbus), which means:
- Credential fallback paths are ALWAYS executed
- Bugs in fallback logic manifest as container-specific failures
- This fix ensures authentication works identically in both native and container environments

### User Experience
- No more repeated device authorization prompts
- Reliable CI test execution
- Consistent `--identity` flag behavior

### Code Quality
- Single source of truth for credential retrieval
- Comprehensive documentation for future developers
- Improved test coverage

## Related Issues

Fixes issues related to:
- Container authentication failures
- SSO token caching
- CI environment hanging
- Identity flag parsing

## Migration Notes

No breaking changes. All changes are backward compatible.

Cache directory location for SSO tokens: `~/.cache/atmos/aws-sso/<provider-name>/token.json`
