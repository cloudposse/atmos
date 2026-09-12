# Fix: `newS3Backend` now authenticates the default identity, not just an explicit `identity:` override

**Date:** 2026-09-09

## Summary

`pkg/component/aws/cloudformation/packaging.go`'s `newS3Backend` only used identity-aware credentials
when `info.Identity` was a non-empty, explicit `identity:` override on the component/stack. The standard,
documented pattern — a default identity (`auth.identities.<name>.default: true`), with no per-component
`identity:` override — left `info.Identity` empty, so the function silently built the S3 client via the
bare ambient AWS SDK credential chain instead. Confirmed live against real AWS: the CloudFormation client
for the same command/stack/default identity authenticated correctly, but the template-upload step failed
with `no EC2 IMDS role found`, because it never reused the same identity resolution the CFN client already
had. Separately, even the explicit-identity path was silently broken in a different way: `newS3Backend`
called `s3store.NewStore(opts)` directly, which never wires `opts.Resolver` into the store's internal
`authResolver` field (only the `pkg/ci/artifact` registry's `NewStore`/`NewBackend` do that via
`SetAuthContext`) — so even a correctly-set `info.Identity` would have fallen back to the ambient chain
the first time the store's deferred client was actually initialized.

## Context

This is a more severe, distinct bug from `2026-08-25-artifact-s3-store-endpoint-override.md`. That earlier
fix addressed `pkg/ci/artifact/s3/store.go`'s identity-aware path missing the `EndpointURL` override once
credentials were already being resolved through the identity path — routing correctly, just to the wrong
endpoint. This fix addresses credential resolution never being attempted at all for the default-identity
case: `newS3Backend` never had a code path that reached identity-aware credentials unless `info.Identity`
was an explicit string, so the identity-aware machinery (and its `EndpointURL` handling from the earlier
fix) was simply never invoked for the far more common default-identity pattern. It only appeared to work
against this repo's own Floci-emulator dev fixture because Floci-based local dev setups typically export
dummy static AWS credentials that the ambient chain happens to pick up — real AWS has no such fallback.

`pkg/component/aws/cloudformation/environment.go`'s `buildAWSConfig`/`awsAuthContextFrom` already show the
correct in-process principle for this component: the CloudFormation client's credentials come from
`info.AuthContext.AWS`, which the auth system populates for the active identity regardless of whether that
identity was selected via an explicit override or a configured default (`pkg/auth/manager.go`'s
`Authenticate` populates `AuthContext` for whichever identity name it authenticated). `newS3Backend` had no
equivalent fallback — it only ever inspected `info.Identity` itself.

`pkg/auth/cloud/aws/setup.go` sets `AWSAuthContext.Profile: params.IdentityName` — i.e. `Profile` always
equals the identity name that was actually authenticated, explicit or default. This is the signal
`newS3Backend` was missing.

## Changes

`pkg/component/aws/cloudformation/packaging.go`:

- Added `activeIdentityName(info)`, which returns `info.Identity` when set, otherwise falls back to
  `info.AuthContext.AWS.Profile` (the identity name of whichever identity — explicit or default — already
  authenticated for this command), otherwise `""` when no identity is active at all (the genuine
  no-auth-configured case, where the ambient chain remains the intended behavior).
- `newS3Backend` now branches on `activeIdentityName(info)` instead of `info.Identity` directly. The
  `AuthManager` type-assertion failure path (fail loudly rather than silently falling back to ambient
  credentials) is preserved and now also covers the default-identity case: if a default identity is active
  but `info.AuthManager` isn't the expected type, the function still errors instead of silently using
  ambient credentials.
- `newS3Backend` now calls `artifact.NewBackend(opts)` (the registry entry point that wires
  `opts.Resolver`/`opts.Identity` into the backend via `SetAuthContext` when the backend implements
  `IdentityAwareBackend`) instead of calling `s3store.NewStore(opts)` directly. This closes the
  separate, deeper bug: constructing the store directly never wired the resolver into the store's
  `authResolver` field at all, silently reaching `s3store`'s own nil-resolver fallback to the ambient chain
  on first real use — for the explicit-identity path too, not just the default-identity path this fix is
  primarily about.
- `opts.Type` corrected from `"s3"` to `"aws/s3"` — the registered factory key in
  `pkg/ci/artifact/s3/store.go`'s `storeName` const. This was previously inert metadata (never looked up)
  because the direct `s3store.NewStore` call bypassed the registry's type-keyed lookup entirely; routing
  through `artifact.NewBackend` makes it a real lookup, so it now has to match.
- Import of `pkg/ci/artifact/s3` changed from the `s3store` alias (used to call `NewStore` directly) to a
  blank import, since the package is now referenced only for its `init()`-time factory registration.

`pkg/component/aws/cloudformation/packaging_test.go`: added
`TestNewS3Backend_DefaultIdentityViaAuthContext` (default identity, no explicit override, resolves via
`AuthContext.AWS.Profile` and constructs successfully through the identity-aware path),
`TestNewS3Backend_DefaultIdentityAuthManagerWrongType_Errors` (fail-loud extended to the default-identity
case), and `TestActiveIdentityName_ExplicitOverridesDefault` /
`TestActiveIdentityName_FallsBackToAuthContext` / `TestActiveIdentityName_NoneActive` (direct unit coverage
of the new resolution helper, including the negative no-identity-active path). All new tests construct the
backend only (no real AWS network call — matches the existing `TestNewS3Backend_WithIdentity` pattern,
whose backend defers auth to first real use).

## Validation

- `go build ./...` — passes.
- `go test ./pkg/component/aws/cloudformation/... ./pkg/ci/artifact/...` — all pass, including the new and
  pre-existing `newS3Backend`/`uploadPackage` tests.
- `GOTOOLCHAIN=go1.26.6 ./custom-gcl run --new-from-rev=origin/main pkg/component/aws/cloudformation/packaging.go pkg/component/aws/cloudformation/packaging_test.go`
  — 0 issues (an initial `godot` finding on a comment starting with a lowercase identifier was fixed).
  Other findings reported by a repo-wide `--new-from-rev` run against `delete.go`/`executor_test.go` are
  from concurrent, unrelated work on this branch and out of scope for this fix.
- Not independently re-verified against live AWS in this pass (no cloud credentials available in this
  environment) — validated by build, unit tests, and lint only. The original bug was confirmed live by the
  reporting field test; the fix mirrors the same `AuthContext`-based mechanism already proven live-correct
  for the CloudFormation client itself (`environment.go`).

## Follow-ups

None.
