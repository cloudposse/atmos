# Fix: retry transient network failures in Aqua registry version fetches

**Date:** 2026-08-11

## Summary

- CI job "Acceptance Tests (macos)" failed
  `TestCLICommands/atmos_toolchain_info_yaml_output`: the golden snapshot
  expected a resolved concrete version (e.g. `0.129.12`) for
  `atmos toolchain info replicatedhq/replicated --output yaml`, but the actual
  output showed `version: latest` and an unresolved `.../download/latest/...`
  asset URL.
- Root cause: `pkg/toolchain/registry/aqua/version.go`'s `fetchVersionFromPage`
  (used by `GetLatestVersion`), `fetchVersionsFromPage` (used by
  `GetAvailableVersionsContext`), and `getLatestTag` each made a single,
  un-retried GitHub API request via `ar.get`/`ar.getWithContext`. A transient
  network hiccup on the CI runner made the request fail, and the failure
  propagated up to `resolveLatestVersion` (`pkg/toolchain/info_helpers.go`),
  which — by design — falls back to the literal string `"latest"` rather than
  erroring the whole command.
- This is the same root-cause class as
  `docs/fixes/2026-08-11-toolchain-info-github-fetch-retry.md` (a different
  `atmos toolchain info` sub-fetch that also had no retry), but a different
  code path: that fix was in `pkg/toolchain/set.go`'s `makeGitHubRequest`,
  this one is in the Aqua registry client.

## Context

Found while investigating the CI failure logs attached alongside two
CodeRabbit review comments on PR #2892 (see
`docs/fixes/2026-08-11-coderabbit-toolchain-retry-static-errors-and-rate-limits.md`
for those). This package already has an established, tested retry pattern
for exactly this class of failure — `getBytes` wraps a GET+status-check+body-read
in `retry.WithPredicate` using `registry.TransientRetryConfig()` and
`registry.IsTransientNetworkError` (retries connection resets, broken pipes,
timeouts, and truncated reads; does not retry non-transient failures like a
404 or malformed response) — but `fetchVersionFromPage`, `fetchVersionsFromPage`,
and `getLatestTag` predated that pattern (or were never migrated to it) and
called `ar.get`/`ar.getWithContext` directly.

## Changes

- `pkg/toolchain/registry/aqua/aqua.go`: added `getBytesWithLinkHeader`, a
  sibling of the existing `getBytes` that also returns the GitHub API
  pagination `Link` header — the paginated release-listing endpoints need it
  to walk to the next page, which plain `getBytes` doesn't expose.
- `pkg/toolchain/registry/aqua/version.go`:
  - `getLatestTag` now calls `ar.getBytes` instead of driving `ar.get` +
    manual status-check + manual body-read itself.
  - `fetchVersionFromPage` and `fetchVersionsFromPage` now call
    `ar.getBytesWithLinkHeader` the same way. Removed the now-unused `io` and
    `net/http` imports.
  - This is purely a call-site change — the same `registry.ErrHTTPRequest`
    sentinel is still wrapped on every failure path, `getWithContext`'s
    existing 403-unauthenticated-retry fallback is preserved (both new
    helpers call `getWithContext` internally), and non-transient failures
    (404s, malformed JSON, empty release lists) still return immediately
    without retrying, matching prior behavior.
- `pkg/toolchain/registry/aqua/aqua_test.go`: added a `flakyThenOKClient`
  mock (`httpClient.Client` implementation failing the first call with a
  connection-reset `net.OpError`, succeeding after) and two regression
  tests — `TestAquaRegistry_FetchVersionFromPage_RecoversFromTransientNetworkError`
  and the `fetchVersionsFromPage` equivalent — asserting exactly one retry
  recovers the version. These reproduce the failure class from the CI log at
  the unit level, since simulating an actual mid-flight connection reset
  against a real `httptest.Server` isn't practical.

## Validation

- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test ./pkg/toolchain/...` (full package tree) — pass, including the 2
  new regression tests and all pre-existing `GetLatestVersion`/
  `GetAvailableVersions` tests (pagination, Link-header parsing, the
  403-unauthenticated-retry fallback, no-releases-found, prefix handling).
- `gofumpt -l` on all changed files — clean.
- Live run: `atmos toolchain info replicatedhq/replicated --output yaml`
  against a freshly built binary resolves a concrete version end-to-end.
- `go test ./tests -run '^TestCLICommands$/atmos_toolchain_info_yaml_output'`
  (with `PATH` pointed at the repo's `./build/atmos`, not a stale
  homebrew-installed binary) — passes.

## Follow-ups

None.
