# Fix: Auth credential chain skipping AssumeRole when target identity credentials are cached

**Date:** 2026-03-23

**Reported by:** Alexander Matveev (9spokes)

## Problem

On EKS ARC (Actions Runner Controller) runners with IRSA, `atmos auth` returns stale cached credentials instead of performing the actual `AssumeRole` API call. Terraform runs with the runner's pod credentials instead of the Atmos-authenticated planner role.

This is the **second bug** affecting IRSA on EKS. The first handles env var scrubbing — removing `AWS_WEB_IDENTITY_TOKEN_FILE`, `AWS_ROLE_ARN`, and `AWS_ROLE_SESSION_NAME` from the subprocess environment so the AWS SDK doesn't prefer pod identity over file-based credentials. This second bug is in the credential chain cache lookup and causes the `AssumeRole` step to be skipped entirely.

### Root Cause

In `findFirstValidCachedCredentials()`, the bottom-up scan finds valid cached credentials at the **last** identity in the chain (the target). The caller `fetchCachedCredentials()` then advances `startIndex` past the end of the chain, causing `authenticateIdentityChain`'s loop to never execute.

**Walkthrough with a 2-element chain `[github-oidc, core-artifacts/terraform]`:**

1. `findFirstValidCachedCredentials()` scans bottom-up, finds valid cached credentials at index 1 (`core-artifacts/terraform`) → returns `1`
2. `fetchCachedCredentials(1)` loads those cached creds and returns `startIndex = 1 + 1 = 2`
3. `authenticateIdentityChain(ctx, 2, cachedCreds)` runs the loop `for i := 2; i < 2` → **loop never executes**
4. The cached credentials are returned as-is, without ever calling `identity.Authenticate()` (the actual `AssumeRole` API call)

The cached credentials at the last index represent the **output** of that step. `fetchCachedCredentials` is designed to return `startIndex + 1` so the **next** step can use them as input. But when there is no next step (last in chain), the index goes past the end and the identity loop is skipped entirely.

This means if the credential file ever contains stale or incorrect credentials (e.g., written by a previous auth step in the same run, or from a provider-level auth that wrote base credentials under the identity's profile), they get returned without validation through the actual AWS STS call.

### Why This Manifests on EKS ARC Runners

On EKS runners with IRSA, the credential file can contain provider-level (OIDC) credentials that were cached during the provider authentication step. When `findFirstValidCachedCredentials` returns the last index, `fetchCachedCredentials` advances past the chain end, and these provider-level credentials are returned as if they were the assume-role credentials — without the actual `AssumeRole` call ever happening.

## Fix

In `findFirstValidCachedCredentials()`, skip cached credentials at the target (last) identity and continue scanning earlier in the chain. This ensures the identity's `Authenticate()` method is always called for the final step.

**Changed file:** `pkg/auth/manager_chain.go`

```go
// After validating credentials are not expired:
if i == len(m.chain)-1 {
    log.Debug("Skipping cached target identity credentials to force re-authentication",
        logKeyChainIndex, i, identityNameKey, identityName)
    continue
}
return i
```

**Behavior with the fix for different chain configurations:**

| Chain | Cache state | `findFirstValidCachedCredentials` returns | Result |
|-------|-------------|------------------------------------------|--------|
| `[provider, assume-role]` | Cache at index 1 (last) | Skip index 1 → check index 0 → if valid, return 0 | AssumeRole executes with provider creds as input |
| `[provider, assume-role]` | Cache only at index 1, nothing at 0 | Skip index 1 → index 0 invalid → return -1 | Full re-auth from provider |
| `[provider, assume-role]` | Cache at index 0 (provider) | Return 0 | Identity chain starts at 1, AssumeRole executes |
| `[provider, permset, assume-role]` | Cache at index 1 (middle) | Return 1 | `fetchCachedCredentials(1)` returns startIndex=2, assume-role step executes |

This aligns with the existing comment on lines 30-33 of `manager_chain.go`:

> "CRITICAL: Always re-authenticate through the full chain, even if the target identity has cached credentials."

## Two Independent Bugs Affecting EKS ARC Runners

There are two independent bugs that both affect EKS ARC runners with IRSA. Both fixes are needed for correct behavior:

1. **IRSA env var scrubbing:** Pod-injected IRSA env vars (`AWS_WEB_IDENTITY_TOKEN_FILE`, `AWS_ROLE_ARN`, `AWS_ROLE_SESSION_NAME`) leak into the subprocess, causing AWS SDK to prefer web identity token auth over the Atmos-managed credential files. Fix: scrub these vars from the subprocess environment via `PrepareShellEnvironment`.

2. **Credential chain cache lookup (this fix):** `findFirstValidCachedCredentials` returns the last chain index, `fetchCachedCredentials` advances past the chain end, and the `AssumeRole` call is skipped entirely. Fix: skip the last index in cache lookup to force re-authentication.

Alexander confirmed that both fixes together resolved the issue for 9spokes.

## Test Coverage

Updated `TestManager_findFirstValidCachedCredentials` in `pkg/auth/manager_test.go`:

- When both `id1` and `id2` have valid credentials, the function now returns `id1` (second-to-last, index 1) instead of `id2` (last, index 2), forcing re-authentication of the target identity.
