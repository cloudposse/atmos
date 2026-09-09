# Fix: `atmos auth logout` now clears the Azure realm MSAL cache so re-login is fresh

**Date:** 2026-09-08

## Summary

`atmos auth logout` (including `--all --force`) left the realm-scoped Azure MSAL token
cache on disk. Because the device-code / interactive providers attempt a **silent** MSAL
token acquisition before any interactive flow, the next `atmos auth login` reused the
previous session's cached account and refresh token — a stale token that predates a role or
PIM change — so newly granted access never took effect and the user kept getting
`403 AuthorizationFailed`. Logout now removes the realm MSAL cache
(`~/.azure/atmos/{realm}/msal_token_cache.json`), forcing a genuine re-authentication, while
preserving the shared Azure CLI cache (`~/.azure/msal_token_cache.json`) that is co-owned
with a user's own `az login` session.

## Context

Reported from the field: an operator elevated their Azure role via PIM, then ran an AKS
integration and got `403` — `"The client '...' does not have authorization to perform action
'Microsoft.ContainerService/managedClusters/read' ... If access was recently granted, please
refresh your credentials."` They ran `atmos auth logout --all --force` (which reported
`Logout all completed`, `Logged out all 4 identities`) and `atmos auth login` again, but the
403 persisted and `atmos auth whoami` showed the credential expiring in ~41 minutes — i.e.
still the **same pre-PIM session**, not a fresh login. The manual workaround was
`rm ~/.azure/atmos/cw/msal_token_cache.json`, after which a fresh login succeeded.

Root cause — two distinct caches, only one cleared on logout:

- **Login** (`deviceCodeProvider.createMSALClient`, `pkg/auth/providers/azure/device_code.go`)
  builds its MSAL public client from `NewMSALCache("", p.realm)` →
  `~/.azure/atmos/{realm}/msal_token_cache.json`. `Authenticate` then calls
  `client.Accounts()` + `trySilentTokenAcquisition()` **before** any interactive flow, so a
  populated realm cache yields a silently-refreshed token from the old session.
- **Logout** (`deviceCodeProvider.Logout` → `deleteCachedToken` → `getTokenCachePath`) only
  removed the Atmos device-code token at
  `~/.cache/atmos/azure-device-code/<provider>/token.json` (an XDG cache path) — a
  **different file**. The realm MSAL cache was never touched, so silent re-login kept
  serving the stale account/refresh token.

The `azure/interactive` provider (the one in the report) embeds `deviceCodeProvider` and
inherits the same `Logout`, so it had the same defect. The `azure/cli` and `azure/oidc`
providers do not create a realm MSAL cache (`NewMSALCache` is only used by the device-code
path), so they are unaffected.

## Changes

- `pkg/auth/cloud/azure/msal_cache.go`
  - Extracted the realm→path logic into `resolveMSALCachePath(realm)` as the single source
    of truth shared by the cache constructor and remover; `NewMSALCache` now delegates to it.
  - Added `RemoveMSALCache(realm)`: deletes `~/.azure/atmos/{realm}/msal_token_cache.json`,
    treating a missing file as success. It **refuses to delete the empty-realm path**
    (`~/.azure/msal_token_cache.json`) because that cache is co-owned with the user's own
    `az login` session — clearing it on an Atmos logout would sign the user out of `az` too.
- `pkg/auth/providers/azure/device_code.go`
  - `deviceCodeProvider.Logout` now removes the realm MSAL cache via
    `azureCloud.RemoveMSALCache(p.realm)` in addition to the existing device-code token
    deletion. This flows through `Logout`, `LogoutProvider`, and `LogoutAll`, which all call
    `provider.Logout(ctx)`, and is inherited by the embedded `interactiveProvider`.
- Tests
  - `pkg/auth/providers/azure/logout_msal_cache_test.go` (new): logout removes the realm MSAL
    cache for both device-code and interactive providers, preserves the shared Azure CLI
    cache, and treats a missing cache as a clean no-op.
  - `pkg/auth/cloud/azure/msal_cache_test.go`: `TestRemoveMSALCache` covers realm-cache
    removal, the missing-file no-op, and the empty-realm safety guard.

## Validation

- Wrote the reproducing tests first and confirmed they **failed** against the unpatched code
  (realm MSAL cache survived logout: `realm MSAL cache should be removed by logout, got
  err=<nil>`), then pass after the fix.
- `go test ./pkg/auth/cloud/azure/ ./pkg/auth/providers/azure/ -count=1` — both packages pass.
- `go test ./pkg/auth/ -run Logout -count=1` — manager-level logout tests pass (they exercise
  the provider `Logout` through `LogoutAll`).
- `go build ./pkg/auth/...`, `go vet ./pkg/auth/cloud/azure/ ./pkg/auth/providers/azure/`, and
  `gofumpt -l` on the changed files are all clean.
- `golangci-lint` was not run from this session (blocked by the sandbox); to be run via the
  `lint` skill / CI before merge.
- Not re-verified end-to-end against a live tenant in this session; the field report plus the
  manual `rm` workaround already established the on-disk root cause, which the tests pin.

## Follow-ups

None.
