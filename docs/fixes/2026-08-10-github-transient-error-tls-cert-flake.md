# Fix: live GitHub API tests treat TLS/network transport failures as transient

**Date:** 2026-08-10

## Summary

`TestIsArchived_LiveNetwork` failed on a Windows Acceptance Tests CI run with
`tls: failed to verify certificate: x509: certificate signed by unknown authority` reaching
`api.github.com`. This is a CI-runner-side TLS trust failure, not a real GitHub API error or a
bug in Atmos. `isGitHubTransientError` — the helper live-network tests in `pkg/github` already use
to skip on conditions outside the test's control (rate limits, GitHub 5xx) — didn't recognize
transport-level failures (DNS, connection, TLS) at all, so this one failed the build instead of
skipping.

## Context

`pkg/github/archived_test.go`'s `TestIsArchived_LiveNetwork` calls the real GitHub API and already
has three layers of defensive gating: `tests.RequireGitHubAccess`, a remaining-rate-limit check,
and `isGitHubTransientError`. The gap: `isGitHubTransientError` only checked for
`errUtils.ErrGitHubRateLimitExceeded`, a `*github.ErrorResponse` with a 5xx status, or a
`"rate limit"` substring — all cases where an HTTP response was actually received. A pure
transport failure (the request never got a response at all) isn't any of those.

Traced the error's path: `getArchivedStatus` (`pkg/github/archived.go`) calls
`handleGitHubAPIError(err, resp)` on failure. `handleGitHubAPIError`'s 401 and rate-limit branches
both require `resp != nil`; when `resp` is `nil` (exactly the transport-failure case), it falls
through to `return err` unwrapped — so the raw `*url.Error` the `net/http` client constructs
reaches the test as-is, never converted to a `*github.ErrorResponse`.

## Changes

- `pkg/github/releases_test.go`: `isGitHubTransientError` now also recognizes `*url.Error` (via
  `errors.As`) as transient — DNS resolution, connection refused/reset, timeouts, and TLS
  certificate trust failures are all wrapped this way by `net/http`, and all represent CI-runner
  networking issues outside the test's control, matching the function's existing stated intent.
  Checked after the `*github.ErrorResponse` case so a genuine API error (which does have a
  response) is never misclassified.
- Added `TestIsGitHubTransientError_TransportFailure`, written first per this repo's test-first
  bug-fixing workflow (confirmed failing before the fix): covers a `*url.Error` wrapping a TLS
  `x509.UnknownAuthorityError` (matching the observed CI failure) and one wrapping a DNS timeout,
  plus a non-regression case confirming a real 404 `*github.ErrorResponse` still classifies as
  non-transient.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/github/... -count=1` — all pass, including the new test individually verified
  via `-v` both before (red) and after (green) the fix.
- `atmos fix lint` (patch-scoped) — 0 issues, after fixing 2 `godot` comment-sentence findings
  (a lowercase identifier — `isGitHubTransientError`/`handleGitHubAPIError` — starting what
  godot read as a new sentence).

## Follow-ups

None.
