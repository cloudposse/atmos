# Fix: replace wall-clock timing assertion with a concurrency-depth check in a flaky provider-mirror test

**Date:** 2026-08-25

## Summary

`TestProviderMirror_VersionResolvesPlatformsConcurrently` (`pkg/terraform/registry`) failed on
Windows CI: `"2.7456633s" is not less than "1.2s"`. The test compared wall-clock elapsed time
against a fraction of a computed serial-execution floor to prove platform downloads run
concurrently, not one at a time -- a comparison already documented in its own prior comment as
having been loosened once before after an earlier flake. It now instruments peak concurrent
in-flight downloads directly with an atomic counter and asserts on that instead, eliminating the
wall-clock dependency entirely.

## Context

Unrelated to the PR under active work (`osterman/fix-workdir-h-char-injection`, PR #2985) --
`pkg/terraform/registry` has zero diff on that branch. Found via an attached
"Acceptance Tests (windows, shard 9/10)" CI failure log while asked to fix failing CI actions.

The test's own comment recorded a prior tuning: an earlier, tighter threshold flaked at 784ms
against a 750ms bound, and was widened to 4/5 of the 10-platform * 150ms serial floor (1.2s). This
run flaked again, and worse: 2.75s exceeds even the *full* serial floor (1.5s) a genuine
regression to one-at-a-time fetching would itself take. That means no static wall-clock threshold
reliably separates "concurrent, but a loaded Windows runner added overhead" from "serial" here --
under enough CI noise, concurrent execution can measure slower than the serial floor's ideal case,
and no amount of threshold-widening fixes that without eventually making the test unable to catch
a real regression either.

The sibling test in the same file, `TestFetchPlatformArchives_BoundsConcurrency`, already used a
load-independent alternative: an atomic high-water-mark counter tracking peak concurrent in-flight
requests, asserted directly rather than inferred from timing. That test operates on
`fetchPlatformArchives` directly, bypassing the real HTTP proxy stack; the flaky test specifically
exercises the full `proxy.NewServer` + `httptest.NewServer` path (per its own comment, guarding
against the original real-world incident: OpenTofu's own mirror-request deadline being exceeded).
The same counter pattern was applied to its fake upstream's download handler instead, preserving
end-to-end coverage of the real server stack while removing the timing dependency.

## Changes

- `pkg/terraform/registry/provider_mirror_test.go`: `TestProviderMirror_VersionResolvesPlatformsConcurrently`
  now tracks peak concurrent in-flight requests to the fake upstream's per-platform download
  handler via an atomic counter (same compare-and-swap high-water-mark pattern already used by
  `TestFetchPlatformArchives_BoundsConcurrency`), and asserts `peak > 1` -- a serial implementation
  can never have more than one download in flight at once, so this directly proves the property
  under test regardless of how slow the scheduler or network happens to be on a given CI run. The
  `elapsed`/`serialFloor` wall-clock comparison was removed entirely rather than further widened.
  Updated the test's doc comment to explain why (documenting the two prior flakes for the next
  person who finds this file).

## Validation

- `go build ./...` -- clean.
- `gofumpt -l pkg/terraform/registry/provider_mirror_test.go` -- clean.
- `go vet ./pkg/terraform/registry/...` -- clean.
- `go test ./pkg/terraform/registry/... -run 'TestProviderMirror_VersionResolvesPlatformsConcurrently|TestFetchPlatformArchives_BoundsConcurrency|TestProviderMirror_' -v -count=5` -- all pass, consistently ~0.3s per run.
- `go test ./pkg/terraform/registry/... -run TestProviderMirror_VersionResolvesPlatformsConcurrently -race -count=3` -- clean, no data races on the new atomic counter.
- `go test ./pkg/terraform/registry/...` (full package) -- pass.
- Patch-scoped `./custom-gcl run --new-from-rev=origin/main` -- 0 issues.

## Follow-ups

None.
