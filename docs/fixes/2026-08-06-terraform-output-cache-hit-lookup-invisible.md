# Fix: cache-hit `!terraform.output` lookups are now visible

**Date:** 2026-08-06

## Summary

A `test.vars` block (or any config) with multiple `!terraform.output`
lookups against the same already-resolved component+stack logged a
"Fetching ..." message only for the first lookup — a cache miss that
triggered a real fetch. Every subsequent lookup for a *different* output
key on that same component+stack was a cache hit and produced no visible
output at all, only a `Debug`-level log line invisible outside
debug/trace logging. A block with nine lookups across two components would
show only two "Fetching ..." lines, making it look like most of the
lookups silently did nothing.

## Context

`pkg/terraform/output/executor.go`'s `GetOutput` and
`GetOutputWithOptions` both cache outputs per stack+component key
(`terraformOutputsCache`, a `sync.Map`): the first successful fetch for a
component+stack stores the *entire* outputs map at once, by design, so
looking up a second output key on the same component+stack never re-fetches.
Before this fix, that cache-hit branch called only
`log.Debug("Cache hit for terraform output", ...)` and returned directly —
skipping the same `outputLookupSucceeded`/`outputLookupFailed` visible
notification (`ui.Success`/`ui.Error` with a checkmark, gated on the
existing spinner-suppression logic) that a real cache-miss fetch always
produces via `GetOutput`'s non-cache path.

## Changes

- `pkg/terraform/output/executor_utils.go`: added `resolveOutputFromCache`,
  which performs the exact same cache lookup as before, but on a hit
  builds the identical `"Fetching %s output from %s in %s"` message a real
  fetch uses and dispatches to `outputLookupSucceeded`/`outputLookupFailed`
  depending on whether `getOutputVariable` errors — the same visible
  success/failure path a cache-miss fetch takes. Returns `nil` (not a
  zero-value result) when the cache holds nothing for the requested key,
  so callers correctly fall through to a real fetch.
- `pkg/terraform/output/executor.go`: `GetOutput` and `GetOutputWithOptions`
  both now call `resolveOutputFromCache` and return its result directly
  when non-nil, instead of inlining the old two-line cache-hit branch.

## Validation

- New regression test `TestExecutor_GetOutput_CacheHitIsVisible`
  (`pkg/terraform/output/executor_test.go`): pre-populates the cache with
  two output keys for one component+stack, performs two `GetOutput` calls
  (neither triggering `DescribeComponent`), and asserts both produce a
  `"Fetching ..."` line in captured UI output.
- Also added direct unit coverage for `resolveOutputFromCache` itself:
  `TestResolveOutputFromCache_MissReturnsNil` (an unpopulated cache key
  returns `nil`, not a zero-value result) and
  `TestResolveOutputFromCache_GetOutputVariableErrorIsVisible` (a cache hit
  whose output-key expression fails to evaluate still surfaces the visible
  `outputLookupFailed` notification, not just the returned error).
- CI-only test flakiness fix, same underlying change: the regression test's
  raw `assert.Contains(uiOutput.String(), "Fetching vpc_id output...")`
  failed on the linux/macOS Acceptance Tests jobs (but passed locally)
  because GitHub Actions sets `CI=true`, which forces color output; the
  markdown-based UI renderer behind `ui.Success` splits the message into
  multiple ANSI-styled runs right at the literal underscore in `vpc_id`,
  without dropping or reordering any visible character. Fixed by stripping
  ANSI (`ansi.Strip`, the same convention already used elsewhere in this
  test suite) before the `assert.Contains` checks. Reproduced locally with
  `CI=true go test ./pkg/terraform/output/...` both before (failing) and
  after (passing) this change.
- `go build ./...` — clean.
- `go test ./pkg/terraform/output/...` (full package) and
  `CI=true go test ./pkg/terraform/output/...` — both pass.
- `atmos lint --changed` — 0 issues.
- Live end-to-end confirmation (not just the mocked unit test): a real
  `terraform apply` against a local backend producing two real outputs,
  followed by three `!terraform.output` lookups (two distinct keys plus a
  repeat) against the same cached component+stack via
  `atmos describe component`, showed all three lookups as visible
  `✓ Fetching ...` lines, including the second and third (cache-hit) calls.

## Follow-ups

None.
