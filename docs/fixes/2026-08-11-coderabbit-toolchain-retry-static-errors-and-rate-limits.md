# Fix: address CodeRabbit review feedback on the toolchain GitHub retry fetch

**Date:** 2026-08-11

## Summary

- CodeRabbit review on PR #2892 flagged two issues in the retry logic added to
  `pkg/toolchain/set.go`'s `makeGitHubRequest` (see
  `docs/fixes/2026-08-11-toolchain-info-github-fetch-retry.md`):
  1. Two `fmt.Errorf` calls used dynamic error roots instead of static errors from
     `errors/errors.go`.
  2. `isRetryableGitHubStatus` decided retryability from the status code alone, so it
     couldn't distinguish a rate-limited `403` from a terminal one, and ignored GitHub's
     `Retry-After`/`X-RateLimit-Reset` guidance entirely.
- Both are fixed. The rate-limit fix is intentionally partial: it correctly classifies
  403s and respects short header-driven waits, but does not block the CLI for GitHub's
  full mandated cooldown (which can run to tens of minutes) — see Changes for why.

## Context

Two review threads on PR #2892 (`PRRT_kwDOEW4XoM6YZj0M`, `PRRT_kwDOEW4XoM6YZjzy`) were
attached by the user with the standard "verify each finding against current code, fix
only still-valid issues, keep changes minimal" instruction. Both findings were verified
against the live file (line numbers matched exactly, not stale) before fixing.

## Changes

- `pkg/toolchain/set.go`:
  - `http.NewRequest` and `client.Do` failures inside `makeGitHubRequest` now wrap
    `errUtils.ErrFailedToCreateRequest` and `errUtils.ErrHTTPRequestFailed` respectively,
    matching the pattern already used in `pkg/pro/api_client.go` and
    `pkg/auth/cloud/aws/console.go`.
  - `isRetryableGitHubStatus` now takes `http.Header` in addition to the status code.
    A `403` is only retryable when `Retry-After` or `X-RateLimit-Remaining: 0` signals a
    genuine rate limit (GitHub returns 403 for both secondary rate limiting and terminal
    authorization failures — only headers distinguish them). New helpers
    `githubSignalsRateLimit`, `githubRetryWaitWithinBudget`, and `githubRetryAfter`
    (Retry-After first, X-RateLimit-Reset fallback, per GitHub's documented rate-limit
    guidance) implement this.
  - Deliberate scope limit: when a header-driven wait exceeds the existing
    `githubRequestRetryMaxDelay` (2s) budget, the request is *not* retried at all — this
    fetch backs a "nice-to-have" available-versions list in `atmos toolchain info` that
    already degrades gracefully to omitting the section on any failure (pre-existing
    behavior, unrelated to this PR). Blocking the user's command for GitHub's real
    mandated cooldown would be worse UX than the existing fast, graceful degradation. When
    a wait *is* within budget, the existing fixed exponential backoff is used rather than
    sleeping the header's exact value — real GitHub cooldowns are never sub-budget in
    practice, so precise header-driven sleep timing was skipped as unnecessary complexity
    for a case that doesn't occur.
  - Two new named constants (`decimalBase`, `bitSize64`) replace magic numbers in the
    `strconv.ParseInt` call for `X-RateLimit-Reset`, matching the existing convention in
    `pkg/yaml/typed.go` and `pkg/duration/duration.go`.
- `pkg/toolchain/set_test.go`: five new `TestMakeGitHubRequestRetry` subtests — a
  rate-limited 403 recovers (via `Retry-After` and via `X-RateLimit-Remaining: 0`), a
  terminal 403 is not retried, a 429 recovers via `X-RateLimit-Reset` when `Retry-After`
  is absent, and a mandated wait exceeding budget fails fast on the first attempt.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/toolchain/...` (full package) — pass, including all 9
  `TestMakeGitHubRequestRetry` subtests.
- `gofumpt -l pkg/toolchain/set.go pkg/toolchain/set_test.go` — clean.
- `./custom-gcl run --new-from-rev=<HEAD>` — 0 issues (pinned to the exact HEAD commit
  rather than the floating `origin/main` ref, per the established practice in this repo's
  session history — `origin/main` moving mid-lint-run previously caused false-positive
  findings in untouched files).

## Follow-ups

None.
