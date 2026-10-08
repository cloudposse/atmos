# Fix: Chaining from a standalone identity fails or bypasses the root's own authentication

**Date:** 2026-10-02

## Summary

Two related defects affected identities that chain from a standalone identity (`via: identity: <standalone>`):

- Whenever the standalone root had no valid cached credentials, the auth manager tried to authenticate the root
  as a *provider* and failed with `provider "<name>" not registered: invalid auth config`.
- Whenever the standalone root had stored credentials, the cache scan treated them as a valid starting point and
  fed them straight to the next step. For an `aws/user` root with MFA, the raw long-lived IAM keys reached
  `sts:AssumeRole` (`AccessDenied: ... is not authorized to perform: sts:AssumeRole`) without a
  `GetSessionToken`/MFA session. An `aws/credential-process` root whose helper returns no `Expiration` was reused
  forever and the helper never ran again, contradicting the documented "runs every time" behavior.

The manager now authenticates a standalone identity at the root of a chain through the `StandaloneIdentity`
interface, always, and never treats the root's stored credentials as a reusable cache entry.

## Context

Chaining from a standalone identity builds the chain `[root-identity, child, ...]`. When no cached credentials
were found anywhere in the chain, `authenticateProviderChain` in `pkg/auth/manager_chain.go` always called
`authenticateWithProvider(m.chain[0])`, even though `m.chain[0]` is an identity, not a registered provider.

The bug was latent:

- `aws/ambient` roots hid it because their `LoadCredentials` re-resolves credentials live, so the root always
  looked cached.
- `aws/user` roots usually hid it because the long-lived keys in the keyring count as non-expiring cached
  credentials. An `aws/user` root configured with YAML keys (no keyring entry) hit it as soon as its session
  files expired.
- The new `aws/credential-process` identity kind (#3248, #1734) would always hit it, because its
  `LoadCredentials` only reads Atmos-managed session files and must stay side-effect free.

Hiding the first defect exposed the second one. `findFirstValidCachedCredentials` scans the chain bottom-up and
`loadCredentialsWithFallback` returns the root's long-lived keyring keys (no session token, no expiration), which
`isCredentialValid` accepts as "valid non-expiring". The scan returned index 0, `fetchCachedCredentials` fed those
raw keys to the child, and the root's own `Authenticate` (session reuse, `GetSessionToken`, MFA) never ran. Verified
live against a real AWS org with an `aws/user` root plus `mfa_arn` and an `aws/assume-role` child. The same
mechanism reused expiration-less `aws/credential-process` credentials indefinitely (the helper ran 0 times across
repeated chained authentications, even after `auth logout`).

## Changes

- `pkg/auth/manager_chain.go`: extracted root authentication into `authenticateChainRoot`. A registered
  provider takes the unchanged path (`PreAuthenticate`, then `authenticateWithProvider`). A root that is not a
  provider but implements `types.StandaloneIdentity` with `IsStandalone() == true` is authenticated by
  `authenticateStandaloneRoot`, which calls `AuthenticateStandalone`. Its credentials are not written to the
  keyring (the standalone identity owns its storage). Anything else still reports "provider not registered".
- `pkg/auth/manager_chain.go`: added `chainRootIsStandalone` (root is not a registered provider and is a
  standalone identity). `findFirstValidCachedCredentials` now stops its bottom-up scan before index 0 for such
  chains, so authentication always starts at the root through `authenticateChainRoot` ->
  `AuthenticateStandalone`, which owns that identity's caching (`aws/user` reuses its unexpired session files with
  no network call or MFA prompt; `aws/credential-process` reuses unexpired file credentials and otherwise re-runs
  the helper). Cached credentials at index 1 and beyond (for example the assume-role step) are still reused.
  Chains rooted at a registered provider and the ambient-root handling are unchanged.
- `pkg/auth/manager_chain_standalone_root_test.go`: repro and negative tests for both defects.
- `pkg/auth/docs/ARCHITECTURE.md` and `website/docs/cli/configuration/auth/identities.mdx`: describe the root
  authentication and caching rule.

## Validation

- Wrote the repro test first; before the fix it failed with
  `provider "root" not registered: invalid auth config`. It passes after the fix.
- Negative tests: a chain rooted at a registered provider never calls the standalone path; an unregistered,
  non-standalone root still fails with "not registered"; a failing standalone root fails the chain without
  running the child.
- Second defect: wrote the tests first. `TestAuthenticateChain_StandaloneRoot_IgnoresCachedLongLivedRootCredentials`
  and `TestAuthenticateChain_StandaloneRoot_ExpiredLaterStepReauthenticatesFromRoot` failed before the fix (the scan
  returned index 0, `AuthenticateStandalone` was never called, the child received the stored long-lived credentials)
  and pass after it. Negative and scoping tests pass before and after:
  `TestAuthenticateChain_ProviderRoot_ReusesCachedProviderCredentials` (provider-rooted chains still reuse cached
  provider credentials), `TestAuthenticateChain_StandaloneRoot_ReusesCachedCredentialsAtLaterStep` (valid cache at
  index 1 is reused and the root is not re-authenticated), and
  `TestAuthenticateChain_StandaloneSingleElementChain_Unchanged`.
- Reuse of an unexpired `aws/user` session without STS or MFA is covered by the existing
  `TestUserIdentity_Authenticate_UsesExistingSessionCredentials`.
- Live with a fake helper that returns no `Expiration` (`aws/credential-process` root, `aws/assume-role` child;
  the child `AssumeRole` is expected to fail, the observable is the helper run counter): before the fix the helper
  ran 0 times per `atmos auth env -i <child> --login`; after the fix it runs exactly once per invocation across
  repeated runs.
- `go test ./pkg/auth/... ./cmd/auth/... ./cmd/aws/...` passes.
- End-to-end coverage exists in `TestAWSCredentialProcessFlociE2E_AssumeRoleChain`, which authenticates an
  `aws/assume-role` chained from an `aws/credential-process` root against the Floci AWS emulator. It is opt-in and
  skips when no emulator is available (or when Floci does not support `sts:AssumeRole`), so it runs in the opt-in
  Floci CI job and was not run for this fix.
- Live against a real AWS account with a disposable IAM user (access key plus virtual MFA device) and a role whose
  trust policy requires `aws:MultiFactorAuthPresent`, chained as `aws/user` (with `mfa_arn`) -> `aws/assume-role`.
  The TOTP codes were generated from the device seed and typed into the real MFA prompt through a pseudo-terminal.
  The user, key, MFA device, and role were deleted afterwards.
  - Keys in the keyring: the `origin/main` build sent the raw IAM keys to `AssumeRole` and got `AccessDenied`. The
    fixed build prompted for MFA once, authenticated the root with `GetSessionToken`, and assumed the role.
  - Keys in YAML: the `origin/main` build failed with `provider "<root>" not registered`. The fixed build prompted
    for MFA once and assumed the role.
  - Non-interactive reruns (`auth exec`, `aws credential-process`, and the AWS CLI through a `credential_process`
    profile) reused the MFA session without prompting.
  - With the cached sessions removed, the AWS CLI with a terminal on stdin and stderr captured failed in about
    two seconds with an error naming the MFA prompt and an `atmos auth login` hint instead of hanging.
- Final combined run on the branch with all of the 2026-10-02 auth fixes applied: `go build ./...`; `go test` for
  `./errors/... ./pkg/auth/... ./cmd/auth/... ./cmd/aws/... ./cmd/azure/... ./cmd/gcp/... ./pkg/devcontainer/...
  ./pkg/store/... ./pkg/process/... ./pkg/io/... ./pkg/data/...`; `go test -short ./cmd/... ./internal/exec/...`;
  `go test ./tests -run 'TestCLICommands/atmos_(auth|aws)'`; `GOOS=windows go vet` on the auth packages; and
  `./custom-gcl run --new-from-rev=origin/main` (0 issues). All passed.

## Follow-ups

None.
