# Fix: address CodeRabbit review feedback on the toolchain GitHub retry fetch

**Date:** 2026-08-11

## Summary

- CodeRabbit review on PR #2892 flagged issues in the retry logic added to
  `pkg/toolchain/set.go`'s `makeGitHubRequest` (see
  `docs/fixes/2026-08-11-toolchain-info-github-fetch-retry.md`):
  - Two `fmt.Errorf` calls used dynamic error roots instead of static errors
    from `errors/errors.go`.
  - `isRetryableGitHubStatus` decided retryability from the status code
    alone, so it couldn't distinguish a rate-limited `403` from a terminal
    one, and ignored GitHub's `Retry-After`/`X-RateLimit-Reset` guidance
    entirely.
  - A follow-up review caught that the fix for the point above only decided
    *whether* to retry — `retry.Do` still applied its own fixed backoff
    regardless of what `Retry-After` actually specified, so a
    `Retry-After: 1` response could be retried after the generic ~300ms
    delay instead of waiting the full second, potentially hitting the same
    rate limit again and wasting the retry budget.
- All three are fixed. `makeGitHubRequest` now sleeps for exactly the
  `Retry-After`/`X-RateLimit-Reset` duration before retrying when one is
  present and within budget, falling back to fixed exponential backoff only
  when no header supplies a wait. A wait exceeding the retry budget still
  fails fast rather than blocking the CLI for GitHub's full mandated
  cooldown (see Changes for why that scope limit remains).

## Context

Four review threads on PR #2892, across two rounds (`PRRT_kwDOEW4XoM6YZj0M`,
`PRRT_kwDOEW4XoM6YZjzy`, then a follow-up pair `PRRT_kwDOEW4XoM6Yasjl`/
`PRRT_kwDOEW4XoM6Yasjm`) were attached by the user with the standard "verify
each finding against current code, fix only still-valid issues, keep changes
minimal" instruction. All findings were verified against the live file (line
numbers matched exactly, not stale) before fixing. The second-round
`Yasjm` thread was a correctness bug in the fix for the first round's
`YZjzy` thread, caught by the same reviewer on a later pass — the initial
implementation computed whether a retry was worthwhile but never actually
plumbed the computed wait duration into when the retry happened.

## Changes

- `pkg/toolchain/set.go`:
  - `http.NewRequest` and `client.Do` failures inside `makeGitHubRequest` now
    wrap `errUtils.ErrFailedToCreateRequest` and `errUtils.ErrHTTPRequestFailed`
    respectively, matching the pattern already used in `pkg/pro/api_client.go`
    and `pkg/auth/cloud/aws/console.go`.
  - `isRetryableGitHubStatus` now takes `http.Header` in addition to the
    status code. A `403` is only retryable when `Retry-After` or
    `X-RateLimit-Remaining: 0` signals a genuine rate limit (GitHub returns
    403 for both secondary rate limiting and terminal authorization
    failures — only headers distinguish them).
  - `makeGitHubRequest` was rewritten from a `pkg/retry.Do`-based call into a
    self-contained loop, because the generic retry executor has no way to
    accept a dynamic, response-derived delay for a specific attempt — it only
    computes backoff from a static config. The loop now calls the new
    `githubRetryAfter` helper (`Retry-After` first, `X-RateLimit-Reset`
    fallback, per GitHub's documented rate-limit guidance) after every
    retryable response and sleeps that exact duration before the next
    attempt; `githubBackoffDelay` (a small local exponential-backoff formula)
    is used only when no header supplies a wait. This is why
    `defaultGitHubRequestRetryConfig` and the `pkg/retry`/`pkg/schema`
    imports were removed — they're no longer used anywhere in the file.
  - Scope limit that remains intentional: when a header-driven wait exceeds
    `githubRequestRetryMaxDelay` (2s), the request is *not* retried at all —
    this fetch backs a "nice-to-have" available-versions list in
    `atmos toolchain info` that already degrades gracefully to omitting the
    section on any failure (pre-existing behavior, unrelated to this PR).
    Blocking the user's command for GitHub's real mandated cooldown (which
    can run to tens of minutes) would be worse UX than the existing fast,
    graceful degradation.
  - Two named constants (`decimalBase`, `bitSize64`) replace magic numbers in
    the `strconv.ParseInt` call for `X-RateLimit-Reset`, matching the
    existing convention in `pkg/yaml/typed.go` and `pkg/duration/duration.go`.
  - Added a trailing period to the rate-limit header block comment (godot).
- `pkg/toolchain/set_test.go`: the `TestMakeGitHubRequestRetry` subtests now
  record a timestamp per request and assert the elapsed time between
  attempts, not just the retry count — this is the regression guard for the
  `Yasjm` thread. The `Retry-After: 1` and `X-RateLimit-Reset` subtests assert the
  gap is at least the header's duration (previously only attempt count was
  checked, which the buggy fixed-backoff version would also have satisfied).
  The headerless-fallback subtests assert the gap matches
  `githubBackoffDelay`.

## Validation

- `go build ./...` — clean.
- `go vet ./pkg/toolchain/...` — clean.
- `go test ./pkg/toolchain/...` (full package) — pass, including all 9
  `TestMakeGitHubRequestRetry` subtests (the `Retry-After: 1` and
  `X-RateLimit-Reset` subtests reproduced the bug — failed against the prior
  implementation, pass against this one).
- `gofumpt -l pkg/toolchain/set.go pkg/toolchain/set_test.go` — clean.
- `./custom-gcl run --new-from-rev=<HEAD>` — 0 issues (pinned to the exact
  HEAD commit rather than the floating `origin/main` ref, per the established
  practice in this repo's session history — `origin/main` moving mid-lint-run
  previously caused false-positive findings in untouched files).

## Follow-ups

None.
