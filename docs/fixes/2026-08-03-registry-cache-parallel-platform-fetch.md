# Fix: Terraform registry cache proxy resolves provider platforms concurrently

**Date:** 2026-08-03

## Summary

The `Screengrabs` GitHub Actions job (run 30823640530, job 91719711270) failed on this branch
with `context deadline exceeded` while OpenTofu queried Atmos's own Terraform Registry Cache
proxy for `hashicorp/aws` provider metadata. Root cause: `ProviderMirror.routeVersion` resolved
every platform a provider version advertises with one upstream HTTP call *at a time* (serially),
and `hashicorp/aws` typically advertises 14-16 platforms — enough cumulative latency on a cold
cache (every CI run starts cold) to exceed OpenTofu's own client-side deadline for the mirror
request. Fixed by resolving all platforms concurrently instead of serially.

## Context

Atmos's Terraform Registry Cache is a local HTTPS proxy that implements the Terraform Provider
Network Mirror Protocol, translating requests into the upstream Provider Registry Protocol
(`pkg/terraform/registry/provider_mirror.go`, introduced in a single recent commit, #2582). To
answer one `<version>.json` mirror request, `routeVersion` fetches the provider's full version
list, then loops over every platform that version advertises and fetches each platform's download
metadata (filename + hash) from the upstream registry — even though the calling client (OpenTofu)
only ever needs the single platform it's running on. Each upstream call has a 30s timeout
(`pkg/http/client.go`) but the loop has no aggregate ceiling, so total wall time scales with
platform count.

This was the *only* failure among the 15 most recent `Screengrabs` runs across all branches
(including the 9 immediately prior runs on this exact branch), so it is not a deterministic bug
that fails every time — but the architecture is a latent risk on every cold-cache run, not a pure
random flake: any upstream latency bump, multiplied across a dozen-plus serial round-trips, can
push total time past the client's mirror-request deadline. Confirmed by reading the code (not
guessing) that the timeout in the log ("context deadline exceeded" on the client's own request to
`127.0.0.1:38003`) originates from OpenTofu's provider-installer client giving up on the local
mirror, not from Atmos's own 30s-per-call upstream timeout ever firing.

## Changes

- `pkg/terraform/registry/provider_mirror.go`: extracted the per-platform resolution loop out of
  `routeVersion` into `fetchPlatformArchives`, which now fans out one goroutine per platform (via
  `sync.WaitGroup` + a buffered results channel) instead of fetching them one at a time. Failed
  platforms are still skipped exactly as before (a platform Terraform doesn't need is allowed to
  fail to resolve; the one it does need surfaces on its own request). Bundled `svc`/`coord`/
  `version` into a new `platformArchiveRequest` struct (passed by pointer) to stay within this
  repo's `revive` argument-count and `gocritic` large-value-by-copy lint rules.
- `pkg/terraform/registry/provider_mirror_test.go`: added
  `TestProviderMirror_VersionResolvesPlatformsConcurrently`, a regression test using a fake
  registry with 10 platforms each taking 150ms to resolve, asserting total wall time stays well
  under the serial floor (10 × 150ms = 1.5s) — directly reproduces and guards against the failure
  class that caused the CI job to fail.

## Validation

- Confirmed the new test fails against the pre-fix serial implementation (temporarily reverted
  `routeVersion` to the old loop): 1.51s elapsed, correctly rejected by the `< 750ms` assertion.
  Restored the fix and reran: 0.16s elapsed, passes.
- `go test ./pkg/terraform/registry/... -race -count=1` — all tests pass, no data races (the
  concurrent fetches share only a channel and a `sync.WaitGroup`; `discovery.resolve`'s cache
  already used a mutex before this change).
- `go build ./...` — clean.
- `./custom-gcl run --new-from-rev=origin/main` (patch-scoped, this repo's real CI lint gate) —
  clean on this patch after two follow-up fixes for `argument-limit` (bundled 3 params into
  `platformArchiveRequest`) and `hugeParam` (pass that struct by pointer). One pre-existing,
  unrelated `cyclomatic complexity` finding on `ActionForMode`
  (`pkg/terraform/tfmigrate/tfmigrate.go`) remains, from an earlier commit this patch does not
  touch.
- Not yet re-run against the actual failed GitHub Actions job — this fix is uncommitted pending
  the user's decision on whether/when to commit and push (per this repo's "never commit without
  being asked" convention). Re-running job 91719711270 (or the workflow as a whole) after pushing
  would confirm the fix resolves the observed failure, but since the original failure was
  intermittent (1 in 15 recent runs), a single successful rerun cannot alone prove the fix -
  the code-level reasoning and the new regression test are the primary evidence.

## Follow-ups

None.
