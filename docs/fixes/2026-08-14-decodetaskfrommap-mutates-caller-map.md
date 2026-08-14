# Fix: `decodeTaskFromMap` no longer mutates the caller's map

**Date:** 2026-08-14

## Summary

`decodeTaskFromMap` (`pkg/schema/task.go`) deleted the polymorphic `with:`
and `container:` keys directly from its input map (`delete(m, "with")` /
`delete(m, "container")`). For a task with no structured `output:`,
`prompt:`, or `steps:` block — the common case — the three normalization
helpers that ran first (`normalizeTaskOutputMap`, `normalizeTaskPromptMap`,
`normalizeTaskStepsMap`) all return their input map unchanged rather than a
copy, so `m` was still the caller's own map. The in-place deletes then
removed `with`/`container` from that shared map, which can be Viper's live
merged config tree, losing the override for any later decode of the same
configuration.

## Context

Flagged by a CodeRabbit review comment on PR #2879 against
`pkg/schema/task.go:866-886`. Verified against current code:
`normalizeTaskOutputMap` returns `m` unchanged when there's no `output` key,
when `output` is a string, or in its default case; `normalizeTaskPromptMap`
returns `m` unchanged when there's no `prompt` key, when `prompt` is a
string, or when it isn't a decodable map; `normalizeTaskStepsMap` returns
`m` unchanged when there's no `steps` key. All three only allocate a copy
when they actually rewrite a field. `decodeTaskFromMap` already had a copy
helper available for exactly this purpose — `withoutTaskMapKey`, used
elsewhere in the same file (`normalizeCastTaskOutput`,
`normalizeParallelTaskOutput`) — but the `with:`/`container:` extraction
used `delete()` directly instead.

## Changes

- `pkg/schema/task.go`: `decodeTaskFromMap` now reassigns `m =
  withoutTaskMapKey(m, "with")` / `m = withoutTaskMapKey(m, "container")`
  instead of `delete(m, "with")` / `delete(m, "container")` — copying `m`
  before removing the key rather than mutating whatever map was passed in,
  regardless of whether an earlier normalize step already copied it.

## Validation

- New regression test, written first per this repo's test-first
  bug-fixing workflow (confirmed failing pre-fix, passing post-fix):
  `TestDecodeTaskFromMap_DoesNotMutateCallerMap` (`pkg/schema/task_test.go`)
  — decodes a task map containing both `with:` and `container:` and no
  `output`/`prompt`/`steps`, then asserts the original map still contains
  both keys after `decodeTaskFromMap` returns.
  - Pre-fix failure: `map[string]interface {}{"type":"shell"} does not
    contain "with"` (and same for `"container"`).
  - Post-fix: `PASS`.
- `go build ./...` — clean.
- `gofumpt -l pkg/schema/task.go pkg/schema/task_test.go` — clean.
- `go test ./pkg/schema/...` (full package) — all pass, no regressions.
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues.

## Follow-ups

None.
