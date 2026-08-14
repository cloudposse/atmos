# Fix: retry the GitHub releases fetch behind `atmos toolchain info`

**Date:** 2026-08-11

## Summary

- CI job "Acceptance Tests (macos)" failed `TestCLICommands/atmos_toolchain_info_shows_atmos-inline_registry`:
  the golden stderr snapshot expected an "Available Versions (latest 10):" section that was missing
  from the actual output.
- Root cause: `atmos toolchain info replicatedhq/replicated` fetches the tool's available versions
  live from the GitHub releases API (`pkg/toolchain/set.go`'s `fetchGitHubVersions` /
  `makeGitHubRequest`). `pkg/toolchain/info_helpers.go`'s `getAvailableVersions` already treats a
  fetch failure as non-fatal by design ("Log error but don't fail - just show no available
  versions"), so a single transient network/TLS hiccup on the CI runner silently produced a
  correct-but-incomplete `info` output instead of a crash — which is exactly what made the golden
  snapshot diverge without any error being surfaced in the log.
- This CI run also showed an unrelated `[mock-linux] tests/fixtures/scenarios/complete` job failing
  at `actions/checkout` with a TLS certificate verification error, in the same time window — pure
  GitHub Actions runner infrastructure flakiness, no repo-side fix possible, not addressed here.

## Context

The user attached CI failure logs and asked to fix them. The checkout TLS failure is infra-only
(confirmed: `.github/workflows/test.yml`'s checkout step has no custom SSL/CA/proxy config, and the
runner is a standard hosted `ubuntu-24.04` image) and was reported back without a code change. The
toolchain-info failure traced to `getAvailableVersions` -> `fetchGitHubVersionsWithSpinner` ->
`fetchGitHubVersions` -> `makeGitHubRequest`, which made a single unauthenticated-by-default,
un-retried HTTP call to `api.github.com`. `github-token` is already wired to read both
`ATMOS_GITHUB_TOKEN` and plain `GITHUB_TOKEN` (see `cmd/root.go`), so this wasn't a rate-limiting
gap — just a missing retry for an otherwise ordinary transient failure, the same class of flakiness
this repo already treats as fixable (see `pkg/oci/pull.go`'s `defaultOCILayerRetryConfig` /
`isRetryableOCILayerError` for the established pattern this fix follows).

## Changes

- `pkg/toolchain/set.go` — `makeGitHubRequest` now wraps the request+response handling in
  `pkg/retry`'s `Do` with a bounded config (3 attempts, exponential backoff from 300ms, capped at
  2s). A new `isRetryableGitHubStatus` helper retries on `429` (rate limited) and `5xx` (server
  error) responses, and lets deterministic client errors (`404`, `403`, etc.) return immediately
  without wasting attempts. Transport-level errors (the TLS/network hiccup class) are always
  retried. `client.Do(req)` picked up a `//nolint:gosec` (G704 SSRF) comment because moving the call
  into the new retry closure made golangci-lint treat the line as newly touched, even though the
  scheme+host (`api.github.com`) was already a hardcoded literal — only the path is user-influenced,
  so no host-redirection risk exists.
- `pkg/toolchain/set_test.go` — new `TestMakeGitHubRequestRetry` covers: recovers after one
  transient 503, recovers after one 429, does not retry a deterministic 404 (asserts exactly 1
  attempt), and exhausts all 3 attempts on a persistent outage. Existing tests
  (`TestFetchGitHubVersions`, `TestFetchGitHubVersionsNetworkEdgeCases`, `TestRealGitHubClient_FetchVersions`)
  go through a separate test-only URL/client path and were unaffected.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/toolchain/...` (full package) — pass, including the new `TestMakeGitHubRequestRetry`
  subtests.
- `gofumpt -l pkg/toolchain/set.go pkg/toolchain/set_test.go` — clean.
- `./custom-gcl run --new-from-rev=<HEAD>` — 0 issues (pinned to HEAD rather than the floating
  `origin/main` ref, which advanced mid-run and briefly surfaced an unrelated pre-existing finding
  in a file this change never touched).

## Follow-ups

None.
