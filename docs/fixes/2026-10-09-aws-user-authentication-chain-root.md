# Fix: Authenticate standalone AWS user chain roots before assuming roles

**Date:** 2026-10-09

## Summary

Fix [#3348](https://github.com/cloudposse/atmos/issues/3348): an `aws/user` identity
at the root of an assume-role chain now authenticates when its session is missing
or expired. Cached long-lived IAM keys cannot bypass session creation and MFA.

## Context

For a chain such as `[ops-user, ops-admin]`, chain construction correctly placed
the user identity first, but authentication treated that step as a provider.
Without cached credentials, login failed with `provider "ops-user" not registered`
before the user identity could prompt for MFA. With long-lived keys in the
keyring, cache validation instead accepted their missing expiration and passed
them directly to the downstream role, skipping `GetSessionToken`.

Historical source inspection found the single-element restriction in the original
auth implementation, [#1475](https://github.com/cloudposse/atmos/pull/1475)
(`96a7852034`, included in `v1.194.0`). The missing-session failure is therefore a
longstanding gap. Root-cache reuse in
[#1729](https://github.com/cloudposse/atmos/pull/1729) (`f12f63d023`, included in
`v1.197.0`) appears to have enabled the long-lived-key bypass. These findings are
based on code and diff inspection, not execution of historical binaries.

## Changes

- Dispatch standalone chain roots through `AuthenticateStandalone`, then pass
  their credentials to downstream identities. Root failures stop the chain.
- Preserve direct standalone login, provider pre-authentication hooks, and reuse
  of valid intermediate sessions.
- Require an AWS session token and parseable expiration for cached `aws/user`
  credentials, applying the existing expiration safety buffer regardless of MFA
  configuration. Revalidate selected credentials after retrieval so a changed
  cache cannot substitute long-lived keys or an expired session.
- Keep session persistence in the user identity and preserve long-lived keyring
  credentials. Shared credential loading and public configuration remain unchanged.
- Add generated standalone-identity mocks and regression tests for missing and
  expired sessions, MFA in YAML or keyring credentials, users without MFA,
  multi-role chains, file-backed session reuse, cache changes, and root failures.
- Correct comments that limited standalone identities to single-element chains.

## Validation

- Ran the regression tests against the pre-fix `manager_chain.go` using a Go
  source overlay. Missing and expired sessions reproduced `provider "user" not
  registered`; all three long-lived-key cases (YAML MFA, stored MFA, and no MFA)
  failed because downstream role authentication received raw IAM keys instead of
  session credentials. The valid-file-session control passed against the old code.
- Focused regression tests passed:
  `go test ./pkg/auth -run 'TestManager_(StandaloneRoot|ChainCredentialEligibility)' -count=1`.
- The complete authentication suite passed: `go test ./pkg/auth/...`.
- Built the repository's custom linter with `go tool mage lint:customGCL` after
  the installed linter reported a missing `lintroller` plugin. Patch-scoped custom
  lint, including the new files, passed with zero issues for `./pkg/auth` and
  `./pkg/auth/types`.
- `gofumpt` and `git diff --check` passed.
- Regression tests use generated mocks; no live AWS calls or interactive MFA
  login were performed.

## Follow-ups

None.
