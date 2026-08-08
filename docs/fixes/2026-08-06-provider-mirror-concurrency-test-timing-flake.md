# Fix: widen the timing margin in the provider-mirror concurrency regression test

**Date:** 2026-08-06

## Summary

`TestProviderMirror_VersionResolvesPlatformsConcurrently`
(`pkg/terraform/registry/provider_mirror_test.go`) failed on the Windows
Acceptance Tests CI job: resolving 10 platforms took 784ms against a 750ms
threshold. The test's own concurrency assertion was actually satisfied —
784ms is nowhere near the 1.5s serial floor the test exists to catch — it
just tripped an overly tight pass/fail bound under normal CI timing
variance. No production code changed; this is a flaky-test fix.

## Context

The test spins up a local HTTP mirror server where each of 10 platform
downloads sleeps a fixed 150ms (`delay`), then asserts that resolving the
version endpoint (which fetches all 10 platform archives) completes in
under half of the theoretical serial floor (`platformCount * delay / 2` =
750ms) — proving the platforms are fetched concurrently (~1 delay) rather
than serially (~10 delays = 1.5s). A concurrent run's actual wall time is
one delay plus scheduling/HTTP/goroutine overhead; on a loaded or
virtualized CI runner — Windows acceptance-test runners in particular are
slower and more variable than local dev machines — that overhead alone can
approach several hundred milliseconds, which the 750ms bound had no real
margin for. This is not related to any other change on this branch;
`pkg/terraform/registry/provider_mirror_test.go` was last touched by
unrelated PRs on `main` (#2534, #2582).

## Changes

- `pkg/terraform/registry/provider_mirror_test.go`: raised the pass
  threshold from `serialFloor/2` (750ms) to `serialFloor*4/5` (1200ms).
  This keeps a wide, unambiguous gap from the 1.5s serial floor a true
  regression to serial resolution would hit, while tolerating realistic CI
  timing variance that a tighter bound doesn't survive.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/terraform/registry/... -run TestProviderMirror_VersionResolvesPlatformsConcurrently -v -count=5`
  — 5/5 pass, each completing in ~310ms (comfortably under the new 1200ms
  bound, comfortably above what a serial regression would need to trip the
  assertion).
- `go test ./pkg/terraform/registry/...` (full package) — pass.
- `atmos lint --changed` — 0 issues.
- Did not reproduce the exact 784ms Windows CI timing locally (this
  machine isn't the flaky runner); the fix targets the assertion's margin,
  not the code path's actual performance, which the test itself already
  confirmed was correct (concurrent, well under the serial floor) even in
  the failing run.

## Follow-ups

None.
