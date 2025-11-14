# PRD: Authentication Session Management - Safe Logout with Optional Keychain Cleanup

**Status:** Draft
**Created:** 2025-11-10
**Author:** Erik Osterman
**Related Commands:** `atmos auth logout`

---

## Executive Summary

This PRD proposes fixing the `atmos auth logout` command to align with industry standards from AWS CLI and gcloud CLI by making the default behavior safe (preserving keychain credentials) while providing an explicit opt-in flag for full cleanup.

Currently, `atmos auth logout` conflates two distinct operations:
1. Clearing **session data** (temporary tokens, cached credentials) - what users expect
2. Deleting **identity credentials** (permanent keys stored in keychain) - often unintended and destructive

This creates user confusion and data loss risk when users simply want to end their current session without permanently deleting credentials.

**Recommended Solution:** Keep the familiar `atmos auth logout` command but add `--keychain` flag for explicit opt-in to credential deletion. Change default behavior to only clear sessions. Use interactive Charm Bracelet (Huh) prompts for confirmation.

---

## Problem Statement

### Current Behavior

When users run `atmos auth logout --all`, the command performs:

1. ✅ **Session cleanup** (desired):
   - Removes cached AWS SSO tokens from `~/.aws/sso/cache/`
   - Deletes temporary AWS credentials from `~/.aws/atmos/<provider>/credentials`
   - Removes provider configuration files from `~/.aws/atmos/<provider>/config`

2. ❌ **Keychain deletion** (often undesired):
   - Permanently deletes IAM user access keys from system keychain
   - Removes service account credentials from keychain
   - Deletes provider credentials (e.g., SAML IdP credentials)

3. ⚠️ **Incomplete cleanup**:
   - Does NOT revoke browser SSO sessions with identity providers
   - Does NOT handle external credential sources (e.g., `GOOGLE_APPLICATION_CREDENTIALS` env var)
   - Does NOT call cloud provider APIs to invalidate server-side sessions

### User Impact

**Scenario 1: Developer switching contexts**
```bash
# Developer wants to test "logged out" behavior
atmos auth logout --all

# Now their IAM user keys are GONE from keychain
# They must re-enter credentials instead of just logging back in
```

**Scenario 2: CI/CD cleanup**
```bash
# CI job cleans up after test run
atmos auth logout --all

# Service account JSON was stored in keychain for reuse
# Now it's deleted and must be re-provisioned
```

**Scenario 3: External credentials**
```bash
# User has GOOGLE_APPLICATION_CREDENTIALS=/path/to/sa.json
atmos auth login gcp-identity

# Later runs logout
atmos auth logout --all

# Keychain is cleared, but environment variable still points to SA file
# Unclear if user is "logged out" or not
```

### Root Cause

The current implementation treats **logout** as a full cleanup operation, when users typically expect:
- **Logout** = "End my session, I'll log back in later with the same credentials"
- **Delete credentials** = "Remove my identity from this machine permanently"

This conflation violates the principle of least surprise and deviates from industry standards.

---

## Industry Research

### AWS CLI

AWS CLI distinguishes between **logout** (local session cleanup) and **revoke** (server-side invalidation):

**aws sso logout:**
- Removes cached SSO access tokens locally
- Deletes temporary AWS credentials from cache
- Does NOT delete IAM user keys from `~/.aws/credentials`
- Does NOT call server-side APIs to invalidate sessions

**Credential revocation:**
- AWS provides separate IAM APIs to revoke role sessions server-side (`sts:RevokeRole`)
- AWS does not revoke individual IAM user access keys via CLI (must use console/API)
- Revocation is server-side and requires admin privileges

**Key insight:** AWS treats logout as **local session cleanup only**, not credential deletion.

### Google Cloud CLI

Google Cloud CLI uses `gcloud auth revoke` for both local and server-side cleanup:

**gcloud auth revoke [ACCOUNT]:**
- Attempts to revoke OAuth2 token on Google's servers
- If server revocation succeeds OR token already revoked, removes credential from local machine
- Clears `~/.config/gcloud/application_default_credentials.json`
- Does NOT touch `GOOGLE_APPLICATION_CREDENTIALS` environment variable or external files

**gcloud auth application-default revoke:**
- Separate command for Application Default Credentials (ADC)
- Revokes ADC token and removes local ADC file
- Independent from `gcloud auth revoke` (different credential types)

**Key insight:** Google uses "revoke" terminology and combines server-side + local cleanup in one operation. The term "revoke" emphasizes credential invalidation, not just session termination.

### OAuth2 / OpenID Connect Standards

**Session termination:**
- **RP-Initiated Logout (OpenID Connect):** Client requests logout from identity provider
- **Front-Channel Logout:** Browser-based logout propagation
- **Back-Channel Logout:** Server-to-server logout notifications

**Token revocation (RFC 7009):**
- Clients POST to provider's revocation endpoint to invalidate tokens
- Server-side operation that immediately invalidates access/refresh tokens
- Required for security when user explicitly revokes access

**Key insight:** Standards distinguish between:
- **Logout** = Terminating a session with an identity provider
- **Revoke** = Invalidating tokens/credentials on the server side

### Analysis Summary

| Tool | Command | Local Cleanup | Server-Side Invalidation | Keychain Deletion |
|------|---------|---------------|-------------------------|-------------------|
| AWS CLI | `aws sso logout` | ✅ Yes | ❌ No | ❌ No |
| gcloud | `gcloud auth revoke` | ✅ Yes | ✅ Yes (OAuth2) | ❌ No |
| gcloud | `gcloud auth application-default revoke` | ✅ Yes | ✅ Yes | ❌ No |
| Atmos (current) | `atmos auth logout` | ✅ Yes | ❌ No | ✅ **YES** (problem!) |

**Conclusion:** No industry-standard tool deletes keychain credentials as part of logout/revoke. These operations focus on **session termination** and **token invalidation**, not **identity removal**.

---

## Proposed Solution

**Keep `atmos auth logout` command with safer defaults:**

```bash
# Safe logout (new default) - clears session data only
atmos auth logout <identity>
atmos auth logout --provider <provider>
atmos auth logout --all

# Full cleanup - add explicit flag to also delete keychain credentials
atmos auth logout <identity> --keychain
atmos auth logout --all --keychain
```

**Benefits:**
- ✅ Keeps familiar `logout` command that everyone understands
- ✅ Default behavior is safe (doesn't delete credentials)
- ✅ Explicit opt-in for destructive operations via `--keychain`
- ✅ Flag name is concise (10 chars) yet clear in context
- ✅ No new commands to learn
- ✅ Aligns with AWS CLI behavior (logout = session cleanup only)
- ✅ Interactive confirmation using Charm Bracelet (Huh) for better UX

**Implementation changes:**
1. Modify `executeAuthLogoutCommand()` to accept `--keychain` flag
2. Change default behavior: skip keychain deletion unless flag is present
3. Update all logout functions to accept `deleteKeychain bool` parameter
4. Add Huh confirmation prompt when `--keychain` is used in TTY (unless `--force`)
5. Auto-skip prompt in non-TTY environments (CI/CD) and show warning

**Why not add `atmos auth revoke` command?**
- "Logout" is universally understood by developers
- Adding a new command creates confusion about which one to use
- Keep it simple - one command with clear flag for destructive operation
- Save "revoke" for future use case if we implement server-side token revocation

---

## Recommendation

**Keep `logout` and add `--keychain` flag**

**Rationale:**
1. **User familiarity:** "Logout" is intuitive and universally understood
2. **Simplicity:** One command is better than two when there's no semantic difference
3. **Industry alignment:** AWS CLI's `logout` also only clears sessions
4. **Concise yet clear:** `--keychain` is shorter than `--delete-keychain` while remaining clear in context
5. **Interactive UX:** Charm Bracelet confirmation provides friendly, safe experience
6. **Future-proof:** Reserves "revoke" for potential server-side revocation features

**Implementation priority:**
- **Phase 1 (v1.x):** Change `logout` default behavior, add `--keychain` flag with Huh prompts
- **Phase 2 (v2.0):** Remove backwards compatibility support if needed

---

## Detailed Design

### Command Structure

```bash
atmos auth logout [identity] [flags]
```

**Arguments:**
- `identity` (optional): Specific identity to logout. If omitted, enters interactive mode.

**Flags:**
- `--provider <provider>`: Logout all identities for a specific provider
- `--all`: Logout all identities and providers
- `--keychain`: Also remove credentials from system keychain (destructive, requires confirmation)
- `--dry-run`: Preview what would be removed without actually deleting
- `--force`: Skip confirmation prompts (useful for CI/CD)

**Examples:**
```bash
# Safe logout (clears session data only) - NEW DEFAULT
atmos auth logout aws-prod-admin

# Full cleanup (session data + keychain credentials)
atmos auth logout aws-prod-admin --keychain

# Logout all identities for a provider (safe)
atmos auth logout --provider aws-sso-prod

# Full cleanup of everything
atmos auth logout --all --keychain

# Preview mode
atmos auth logout --all --dry-run

# With confirmation bypass for CI/CD
atmos auth logout --all --keychain --force
```

### Behavior Changes

#### Current `logout` behavior
```go
// manager.Logout() performs:
1. credentialStore.Delete(identityName)  // Deletes from keychain (ALWAYS)
2. provider.Logout()                      // Clears session files
3. fileManager.Cleanup()                  // Removes cached files
```

#### New `logout` default behavior
```go
// manager.Logout(deleteKeychain bool) performs:
1. provider.Logout()                      // Clears session files
2. fileManager.Cleanup()                  // Removes cached files
3. if deleteKeychain {
     credentialStore.Delete(identityName) // Only if --keychain flag set
   }
```

### Keychain Cleanup Scope

When `--keychain` is specified, delete from keychain:

**For identity revoke:**
- Identity-specific credentials stored as `atmos-auth:<identity-name>`

**For provider revoke:**
- Provider-level credentials stored as `atmos-auth:<provider-name>`
- All identity credentials that use this provider (transitive via `Via` chain)

**For revoke all:**
- All `atmos-auth:*` entries in keychain

### External Credential Handling

**Google ADC via environment variable:**
```bash
# User has set:
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/sa.json

# Revoke does NOT:
# - Unset the environment variable
# - Delete the file at /path/to/sa.json

# Revoke DOES:
# - Clear any ADC credentials cached by Atmos in ~/.config/gcloud/
# - Warn user if GOOGLE_APPLICATION_CREDENTIALS is set
```

**Warning message:**
```
✓ Logged out gcp-identity (session data cleared)

⚠ Warning: External credentials may still be active:
  • GOOGLE_APPLICATION_CREDENTIALS=/path/to/sa.json

  To fully logout, consider:
  • Unsetting the environment variable
  • Removing the credentials file
```

### Confirmation Prompt

When `--keychain` is used in an interactive terminal (TTY) without `--force`, show Huh confirmation:

**Interactive prompt (using `huh.NewConfirm()`):**
```go
// Use Atmos Huh theme
t := utils.NewAtmosHuhTheme()

// Build prompt message
message := fmt.Sprintf(
  "Delete keychain credentials for %s?\n\n" +
  "This will permanently remove:\n" +
  "  • IAM user access keys\n" +
  "  • Service account credentials\n\n" +
  "Session data will also be cleared.",
  identityName,
)

// Create confirmation prompt
confirmPrompt := huh.NewConfirm().
  Title(message).
  Affirmative("Yes, delete credentials").
  Negative("No, keep credentials").
  Value(&confirm).
  WithTheme(t)

if err := confirmPrompt.Run(); err != nil {
  return fmt.Errorf("confirmation prompt failed: %w", err)
}

if !confirm {
  ui.Info("Logout cancelled - credentials preserved")
  return nil
}
```

**Visual appearance:**
```
? Delete keychain credentials for aws-prod-admin?

  This will permanently remove:
    • IAM user access keys
    • Service account credentials

  Session data will also be cleared.

  > Yes, delete credentials
    No, keep credentials
```

**Non-TTY behavior (CI/CD):**
When `--keychain` is used but stdin is not a TTY:
```
⚠ Warning: --keychain specified but not in interactive terminal
  Keychain deletion requires confirmation. Options:
  • Use --force to bypass confirmation in CI/CD
  • Run interactively to confirm deletion

✗ Logout cancelled - use --force to delete keychain in non-interactive mode
Exit code: 1
```

### Migration Path

**Phase 1: Change default behavior (v1.x.x) - BREAKING CHANGE**
```bash
# New default behavior (safe)
atmos auth logout <identity>
⚠ Note: Behavior changed in v1.x.x
        Logout now only clears session data by default.
        To also delete keychain credentials, use: --keychain

# Full cleanup (opt-in, with interactive confirmation)
atmos auth logout <identity> --keychain
```

**Migration announcement:**
- Blog post explaining the change
- CHANGELOG entry with prominent notice
- Update all documentation
- Add temporary CLI hint on first use after upgrade

**Phase 2: Remove migration hint (v1.x+1.0)**
```bash
# Remove the migration hint after one minor version
atmos auth logout <identity>  # No warning, just works
```

### Error Handling

**Existing error types (reused):**
```go
ErrLogoutFailed         = errors.New("failed to logout")
ErrPartialLogout        = errors.New("partial logout completed")
ErrLogoutNotSupported   = errors.New("logout not supported for this provider")
ErrCredentialDeleteFailed = errors.New("failed to delete credentials from keychain")
```

**Partial logout behavior:**
```bash
atmos auth logout --all --keychain --force

# If some operations fail:
✓ Logged out aws-prod-admin (session data cleared)
✗ Failed to delete aws-prod-admin credentials from keychain: access denied
✓ Logged out gcp-identity (session data cleared)
✓ Deleted gcp-identity credentials from keychain

⚠ Partial logout completed. Some operations failed.
  Exit code: 0 (treated as success)
```

### Testing Requirements

**Unit tests:**
- Test logout without `--keychain` (keychain untouched)
- Test logout with `--keychain` in TTY (shows Huh prompt)
- Test logout with `--keychain` in non-TTY (requires --force)
- Test logout with `--keychain --force` (bypasses prompt)
- Test Huh confirmation accept/reject flows
- Test partial logout scenarios
- Test external credential detection and warnings
- Test dry-run mode
- Test provider cascade (logging out provider logs out dependent identities)

**Integration tests:**
- Test actual keychain operations (using test keychain backend)
- Test file cleanup for each provider (AWS SSO, SAML, GitHub OIDC)
- Test interactive mode for logout command

**Snapshot tests:**
- Capture output for each logout scenario
- Verify warning messages for external credentials
- Verify non-TTY warning when `--keychain` used without `--force`
- Verify migration hints
- Note: Huh prompts are interactive and not suitable for snapshot tests

---

## Documentation Updates

### CLI Documentation

**Update page:** `website/docs/cli/commands/auth/auth-logout.mdx`

Update with new behavior:

```markdown
---
title: atmos auth logout
description: Logout from authenticated identities or providers
slug: /cli/commands/auth/auth-logout
---

# atmos auth logout

End your session by clearing session data (tokens, cached credentials) for one or more identities.

:::info Behavior Change in v1.x.x
Starting in v1.x.x, `auth logout` only clears session data by default.
Keychain credentials are preserved for future logins.

To also delete credentials from the system keychain, use the `--keychain` flag.
:::

## Usage

```bash
atmos auth logout [identity] [flags]
```

## Examples

```bash
# Logout (session data only) - RECOMMENDED
atmos auth logout aws-prod-admin

# Logout and delete keychain credentials (prompts for confirmation)
atmos auth logout aws-prod-admin --keychain

# Logout all identities for a provider
atmos auth logout --provider aws-sso-prod

# Logout everything and delete all credentials (prompts for confirmation)
atmos auth logout --all --keychain

# Preview what would be removed
atmos auth logout --all --dry-run

# CI/CD usage - bypass confirmation
atmos auth logout --all --keychain --force
```

## Flags

<dl>
  <dt>`--provider <provider>`</dt>
  <dd>Logout all identities that use this provider</dd>

  <dt>`--all`</dt>
  <dd>Logout all identities and providers</dd>

  <dt>`--keychain`</dt>
  <dd>Also remove credentials from system keychain (destructive operation, requires interactive confirmation or --force)</dd>

  <dt>`--dry-run`</dt>
  <dd>Preview what would be removed without actually deleting</dd>

  <dt>`--force`</dt>
  <dd>Skip confirmation prompts (useful for CI/CD)</dd>
</dl>

## What Gets Logged Out

**Session data (always removed):**
- Cached tokens (AWS SSO tokens, OAuth2 tokens)
- Temporary credentials (AWS credentials files)
- Provider configuration files

**Keychain credentials (only with `--keychain`):**
- IAM user access keys
- Service account credentials
- Provider credentials (SAML, IdP)

**Not removed:**
- Browser sessions with identity providers (logout from provider website separately)
- External credential files (e.g., files referenced by environment variables)

## When to Use `--keychain`

Use the `--keychain` flag only when:
- Decommissioning a machine
- Removing access for a former team member
- Cleaning up test credentials permanently

For normal usage, logout without `--keychain` to preserve your credentials for the next login.

## Related Commands

- [`atmos auth login`](/cli/commands/auth/auth-login) - Authenticate with a provider
- [`atmos auth list`](/cli/commands/auth/auth-list) - List authenticated identities
```

### User Guide

**Update section:** "Understanding Logout and Sessions"

```markdown
## Session Management

Starting in v1.x.x, `atmos auth logout` has safer default behavior:

- **Logout (default):** Clears temporary session data so you're no longer authenticated. Your credentials remain in the keychain for future logins.
- **Logout with `--keychain`:** Permanently removes both session data and credentials from the system keychain (requires confirmation).

Most users want to **logout** without deleting credentials.

Use `--keychain` only when:
- Decommissioning a machine
- Removing access for a former team member
- Cleaning up test credentials permanently

### Example Workflow

```bash
# Login and save credentials to keychain
atmos auth login aws-prod-admin

# Work with AWS resources
atmos terraform plan

# End session (credentials stay in keychain)
atmos auth logout aws-prod-admin

# Next day - quick login (uses saved credentials)
atmos auth login aws-prod-admin
```
```

### Migration Guide

**New document:** `website/docs/migrations/v1.x-logout-changes.md`

```markdown
---
title: v1.x Authentication Logout Changes
description: Migration guide for auth logout behavior change in Atmos v1.x
---

# v1.x Authentication Logout Changes

## Summary

Atmos v1.x changes the default behavior of `atmos auth logout` to align with industry standards (AWS CLI, gcloud CLI).

**What changed:**
- `auth logout` now only clears session data by default (no longer deletes keychain credentials)
- New `--keychain` flag explicitly controls credential deletion
- Interactive confirmation prompt (Charm Bracelet Huh) added when using `--keychain`

## Migration

**Before (v1.x-1):**
```bash
# Cleared session data AND deleted keychain credentials
atmos auth logout --all
```

**After (v1.x):**
```bash
# Only clear session data (NEW DEFAULT - safe)
atmos auth logout --all

# Clear session data AND delete keychain credentials (opt-in, prompts for confirmation)
atmos auth logout --all --keychain
```

## Impact on CI/CD

If your CI/CD pipelines relied on `logout` deleting keychain credentials, update them:

```bash
# Old behavior
atmos auth logout --all

# New equivalent (requires explicit flag and --force for non-interactive)
atmos auth logout --all --keychain --force
```

The `--force` flag bypasses the confirmation prompt for non-interactive environments.

## Why This Change?

Users reported confusion and data loss when `logout` deleted their permanent credentials. This change:
- Aligns with AWS CLI and gcloud CLI behavior (logout = session cleanup only)
- Makes the default safe (preserves credentials)
- Provides explicit opt-in for destructive operations
- Reduces risk of accidental credential deletion

## Questions?

See the updated [`atmos auth logout`](/cli/commands/auth/auth-logout) documentation for more details.
```

---

## Implementation Plan

### Phase 1: Core Implementation (Sprint 1)

**Tasks:**
1. Modify `cmd/auth_logout.go` to accept `--keychain` flag
2. Update `pkg/auth/manager_logout.go`:
   - Add `deleteKeychain bool` parameter to `Logout()`, `LogoutProvider()`, `LogoutAll()` functions
   - Change default behavior to NOT delete keychain
   - Only delete from keychain if `deleteKeychain == true`
3. Update `pkg/auth/providers/*/` logout methods to handle new behavior
4. Add Huh confirmation prompt when `--keychain` is used in TTY (unless `--force`)
   - Use `utils.NewAtmosHuhTheme()` for consistent styling
   - Use `huh.NewConfirm()` with clear affirmative/negative options
5. Add TTY detection and warning for non-interactive `--keychain` usage
6. Add external credential detection (check env vars like `GOOGLE_APPLICATION_CREDENTIALS`)
7. Update CLI help text and usage examples

**Deliverables:**
- Modified `atmos auth logout` command with new default behavior
- Unit tests with >80% coverage
- Integration tests

### Phase 2: Migration Support (Sprint 1)

**Tasks:**
1. Add temporary migration hint on first logout after upgrade
2. Cache hint display using Atmos cache to show only once per environment
3. Add CLI hints in help text about the behavior change

**Migration hint:**
```
⚠ Note: Starting in v1.x.x, logout only clears session data by default.
        To also delete keychain credentials, use: --keychain
        (This message will only be shown once)
```

**Deliverables:**
- Migration hints in place
- One-time display mechanism tested

### Phase 3: Documentation (Sprint 1-2)

**Tasks:**
1. Update `auth-logout.mdx` documentation page with new behavior
2. Create migration guide (`v1.x-logout-changes.md`)
3. Update user guide with session management explanation
4. Update CLI help text
5. Create blog post announcing the change

**Deliverables:**
- Complete documentation
- Migration guide published
- Blog post ready

### Phase 4: Testing & Validation (Sprint 2)

**Tasks:**
1. Generate golden snapshots for all revoke scenarios
2. Test keychain operations on all platforms (macOS, Linux, Windows)
3. Test external credential detection
4. Validate deprecation warnings
5. Manual QA testing

**Deliverables:**
- All tests passing
- QA sign-off

### Phase 5: Release (Sprint 2)

**Tasks:**
1. Update CHANGELOG.md with prominent BREAKING CHANGE notice
2. Create release notes highlighting behavior change
3. Tag v1.x.0 release
4. Publish blog post announcing the change
5. Update examples repository if needed
6. Notify users via GitHub Discussions / Discord

**CHANGELOG entry format:**
```markdown
## [1.x.0] - YYYY-MM-DD

### BREAKING CHANGES

- **auth logout**: Changed default behavior to only clear session data. Keychain credentials are now preserved by default.
  - To delete keychain credentials, use the new `--keychain` flag
  - Interactive confirmation prompt (Charm Bracelet Huh) required when using `--keychain`
  - Add `--force` to bypass confirmation prompts in CI/CD
  - See [migration guide](/migrations/v1.x-logout-changes) for details

### Added

- Add `--keychain` flag to `atmos auth logout` for explicit credential deletion
- Add interactive Huh confirmation prompt when using `--keychain` in TTY
- Add TTY detection and helpful error for non-interactive `--keychain` usage
- Add detection and warnings for external credentials (e.g., `GOOGLE_APPLICATION_CREDENTIALS`)
```

**Deliverables:**
- Released version with behavior change
- Public announcement via multiple channels
- Clear migration documentation

---

## Success Metrics

**User satisfaction:**
- Reduction in GitHub issues related to lost credentials
- Positive feedback on deprecation warnings and migration path

**Adoption:**
- Usage of `--keychain` flag (should be low - indicates explicit destructive intent)
- Confirmation prompt acceptance rate (track how often users confirm vs cancel)

**Code quality:**
- Test coverage >80% for new code
- Zero critical bugs in first 30 days post-release

---

## Open Questions

1. **Should we implement server-side revocation (calling cloud provider APIs)?**
   - AWS: No API for revoking temporary credentials (STS tokens expire naturally)
   - Google: OAuth2 revocation endpoint available but adds complexity
   - **Decision needed:** Defer to future PR, focus on local cleanup first

2. **Should we provide a way to cleanup external credential files?**
   - Example: Delete file at `$GOOGLE_APPLICATION_CREDENTIALS`
   - Risk: Deleting user files outside Atmos's control
   - **Decision needed:** No - only warn about external credentials, don't touch them

3. **Should `--keychain` require confirmation?**
   - Pro: Prevents accidental deletion
   - Con: Adds friction for intentional deletion
   - **Decision:** Yes - use Huh interactive prompt in TTY, require `--force` in non-TTY

4. **How to handle transitive identity chains?**
   - Example: `identity-a` → `identity-b` → `provider-c`
   - Current: Revoking provider-c revokes both identities
   - **Decision needed:** Keep current behavior, document clearly

---

## Risks & Mitigations

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Users expect old behavior (delete keychain) | High | High | Clear migration notice, one-time hint, blog post, release notes |
| Breaking change breaks CI/CD pipelines | High | Medium | Clear migration guide, document `--keychain --force` pattern, helpful non-TTY error |
| Users don't see migration hint | Medium | Low | Show hint on first use, include in release notes |
| Keychain operations fail silently | High | Low | Comprehensive error handling, integration tests |
| External credentials not detected | Medium | Low | Support common patterns, document edge cases |
| Confusion about when to use `--keychain` | Medium | Medium | Clear documentation, helpful Huh confirmation prompts |
| Huh prompt not working in some terminals | Low | Low | Test across common terminals, fallback to text prompt if Huh fails |

---

## Appendix

### Terminology

**Session data:**
- Temporary tokens (AWS SSO token, OAuth2 access token)
- Cached credentials (AWS credentials file)
- Configuration files (AWS config file)

**Keychain credentials:**
- IAM user access keys (long-term)
- Service account keys (long-term)
- Provider credentials (IdP credentials)

**External credentials:**
- Files referenced by environment variables
- Credentials not managed by Atmos

### Related Issues

- (Add GitHub issue links here)

### References

- [AWS CLI SSO Logout](https://docs.aws.amazon.com/cli/latest/reference/sso/logout.html)
- [gcloud auth revoke](https://cloud.google.com/sdk/gcloud/reference/auth/revoke)
- [OAuth 2.0 Token Revocation (RFC 7009)](https://datatracker.ietf.org/doc/html/rfc7009)
- [OpenID Connect Session Management](https://openid.net/specs/openid-connect-session-1_0.html)
- [OpenID Connect RP-Initiated Logout](https://openid.net/specs/openid-connect-rpinitiated-1_0.html)
