# Fix: retry rate-limit and server-error HTTP statuses in Aqua registry fetches

**Date:** 2026-08-12

## Summary

- CodeRabbit review on PR #2892 flagged that
  `pkg/toolchain/registry/aqua/aqua.go`'s `getBytes`/`getBytesWithLinkHeader`
  retry predicate only accepted `registry.IsTransientNetworkError` (connection
  resets, timeouts, truncated reads). A `429` or `5xx` response was converted
  into a plain `registry.ErrHTTPRequest`-wrapped error, which the predicate
  doesn't recognize as retryable — so these responses returned after a single
  attempt, exactly the class of failure the retry was added to fix (see
  `docs/fixes/2026-08-11-aqua-registry-version-fetch-retry.md`).
- Fixed by classifying `429`, a rate-limited `403`, and `5xx` as retryable,
  reusing the same header-aware classification already built for
  `pkg/toolchain/set.go`'s GitHub retry fix rather than reimplementing it.

## Context

Found via a CodeRabbit review comment (thread `PRRT_kwDOEW4XoM6YcSoD`) on the
retry fix committed the same day. The requested fix overlapped heavily with
logic already written for `pkg/toolchain/set.go` (`isRetryableGitHubStatus`,
`githubSignalsRateLimit`, `githubRetryAfter`): both needed to distinguish a
rate-limited `403` (GitHub returns 403 for both secondary rate limiting and
terminal authorization failures — only `Retry-After`/`X-RateLimit-Remaining`
distinguish them) from a terminal one. Per this repo's "extend, don't fork
abstractions" convention, that logic was extracted into
`pkg/toolchain/registry` — already home to the sibling
`IsTransientNetworkError`/`TransientRetryConfig` — instead of being
duplicated a second time in the `aqua` package.

## Changes

- `pkg/toolchain/registry/githubratelimit.go` (new): `IsRetryableGitHubStatus`,
  `GitHubSignalsRateLimit`, and `GitHubRetryAfter`, moved here verbatim
  (renamed to exported) from `pkg/toolchain/set.go`. Also adds
  `HTTPStatusError`, a typed error carrying a response's status code and
  headers so a `retry.WithPredicate` predicate can classify it via
  `errors.As` instead of parsing the error message string.
- `pkg/toolchain/set.go`: removed its local copies of the three functions and
  the header-name/parsing constants; `makeGitHubRequest` now calls
  `registry.IsRetryableGitHubStatus`/`registry.GitHubRetryAfter`. No
  behavior change — `set.go` keeps its own bespoke retry loop (unlike `aqua`,
  it honors the exact `Retry-After`/`X-RateLimit-Reset` wait rather than a
  generic backoff, and gives up fast when that wait exceeds its small
  budget — see the 2026-08-11 CodeRabbit fix-log for why).
- `pkg/toolchain/registry/aqua/aqua.go`: `getBytes` and `getBytesWithLinkHeader`
  now construct a `*registry.HTTPStatusError` (instead of a plain
  `fmt.Errorf`) for non-200 responses, and pass a new
  `isRetryableAquaFetchError` predicate to `retry.WithPredicate` —
  `registry.IsTransientNetworkError(err) || (errors.As into *HTTPStatusError
  && registry.IsRetryableGitHubStatus(...))`. Both response bodies are still
  closed via the existing `defer resp.Body.Close()`, satisfied before the
  typed error is even constructed. `version.go`'s three call sites
  (`getLatestTag`, `fetchVersionFromPage`, `fetchVersionsFromPage`) needed no
  changes — they already route through `getBytes`/`getBytesWithLinkHeader`
  from the prior fix, so they inherit this one automatically. Unlike
  `set.go`, `aqua`'s retry uses the existing generic
  `registry.TransientRetryConfig()` backoff (5 attempts, 1-10s) rather than
  honoring the exact `Retry-After` value — CodeRabbit's finding asked only
  for these statuses to become retryable at all, not for exact-wait timing,
  and the existing generous budget already accommodates typical GitHub
  cooldowns better than `set.go`'s small one.
- `pkg/toolchain/registry/githubratelimit_test.go` (new): table-driven tests
  for `IsRetryableGitHubStatus`, `GitHubSignalsRateLimit`, `GitHubRetryAfter`,
  and `HTTPStatusError` — direct unit coverage for the now-shared functions
  (previously only covered indirectly via `set_test.go`).
- `pkg/toolchain/registry/aqua/aqua_test.go`: a table-driven
  `TestIsRetryableAquaFetchError` (transient network error, 429, rate-limited
  403, 5xx, terminal 403, terminal 404, unrelated error — the exact matrix
  CodeRabbit requested) plus an end-to-end
  `TestAquaRegistry_GetBytes_RetriesTransientHTTPStatuses` against a real
  `httptest.Server`, confirming the retry actually happens (not just that the
  predicate classifies correctly).

## Validation

- `go build ./...` — clean.
- `go vet ./pkg/toolchain/...` — clean.
- `go test ./pkg/toolchain/...` (full package tree) — pass, including all new
  tests.
- `gofumpt -l` on all changed/new files — clean.
- Caught and fixed a test-construction bug while writing the new unit tests:
  `http.Header{"X-RateLimit-Remaining": {"0"}}` map literals don't match
  `Get()` lookups, because Go canonicalizes header keys (`X-RateLimit-Remaining`
  canonicalizes to `X-Ratelimit-Remaining`, lowercase `atelimit`) — a
  `.Set()`-based helper is used instead in both new test files. Confirmed via
  the end-to-end `httptest.Server` test (which does use `.Set()`) passing
  while the affected direct-unit-test cases initially failed — the production
  code was correct throughout; only the test literals were wrong.

## Follow-ups

None.
