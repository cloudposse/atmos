# Fix: nondeterministic data loss when a deferred YAML function's parent path and a sibling deferred function's child path collide

**Date:** 2026-08-07

## Summary

- Field-testing PR #2892 (the fix for #2888, deferred YAML functions losing data on merge)
  surfaced a still-nondeterministic corruption case not covered by that PR's own regression
  tests: a deferred function occupying a parent path in one merge layer (e.g. `vars.combo:
  !template ...`) competing with a different deferred function occupying a child key of that
  same path in another layer (e.g. `vars.combo.nested: !template ...`).
- Live reproduction against the `atmos-yaml-functions-merge` fixture showed roughly 40% of runs
  of the exact same `atmos describe component` command producing wrong output — either the
  child's value silently dropped to `null`, or the child's raw, still-unresolved function-tag
  string leaking straight into command output — with no error and exit code 0 every time.
- Root cause: `pkg/merge.ApplyDeferredMerges` iterated `dctx.GetDeferredValues()` (a plain Go
  `map[string][]*DeferredValue`) directly. Go randomizes map iteration order per `range`, so the
  parent and child paths could be processed in either order. Each path's resolution ends in an
  unconditional `SetValueAtPath` call that replaces whatever currently exists at that exact
  path — if the ancestor path was processed *after* the descendant, the ancestor's wholesale
  replace of the shared parent map silently discarded the descendant's already-resolved value.
- Fixed by sorting paths ancestor-before-descendant (by ascending path-segment length) before
  processing, so descendant leaf writes always happen last and can never be clobbered by a later
  ancestor replace. This generalizes to arbitrary nesting depth, not just the 2-level case that
  exposed it.

## Context

Investigating via `/field-test` on PR #2892 (branch `osterman/field-test-issue-2888`) at the
user's request — an investigation-only pass per that skill's mandate, so no code changed during
that pass itself. The pass built 8 new fixture components in
`tests/fixtures/scenarios/atmos-yaml-functions-merge/` covering gaps neither PR #2892's unit
tests nor its integration tests exercised (list-nested functions, 3-layer type flips,
backend-section functions, untracked/excluded functions colliding with overrides, a scalar
overriding a map-producing function's own leaf, a parent/child nested-function collision, and a
default-list-strategy mirror of the labels/tags case). All but the nested-function-collision case
were confirmed correct via live execution. The nested-function-collision case reliably reproduced
data corruption; the user then explicitly asked for a regression test and a fix, which is this
change.

## Changes

- `pkg/merge/merge_yaml_functions.go` — `ApplyDeferredMerges` now collects the deferred context's
  path keys into a slice and sorts it by ascending `len(Path)` before the processing loop, instead
  of ranging over the map directly. Ancestor paths (shorter) are always resolved before descendant
  paths (longer) sharing the same prefix.
- `pkg/merge/merge_deferred_test.go` — new subtest under `TestApplyDeferredMerges`, "resolves
  deterministically when a parent path and a child path both defer functions". Runs the
  parent/child collision scenario 200 times via `MergeWithDeferred` + `ApplyDeferredMerges` with a
  real mock processor, asserting the exact correct deep-merged result every iteration. Chosen
  because a single run can pass by chance depending on the (previously) random map iteration
  order — the old code failed intermittently within the first ~15 iterations of 200; the fix
  passes all 200 every run.
- `tests/fixtures/scenarios/atmos-yaml-functions-merge/stacks/catalog/base.yaml` and
  `.../stacks/test-deferred-merge.yaml` — kept the 8 new components from the field-test pass
  (additive only, nothing removed or modified). `base-component-nested-collision` /
  `test-nested-function-collision` are the fixture pair for this specific bug; the other 7 cover
  scenarios that were already confirmed correct and are kept for their standing regression value.

## Validation

- `go test ./pkg/merge/...` (full package) — pass, includes the new regression test.
- `go test ./internal/exec/... -run 'Deferred|YamlFunc|StackProcessorMerge|ProcessStacks'` — pass.
- `go test ./tests/... -run 'TestYAMLFunctionsDeferredMerge|TestDeferredMergeTypeConflictResolution'`
  — pass, including the pre-existing `TestYAMLFunctionsDeferredMergeCacheCorrectness`.
- `gofumpt -l pkg/merge/merge_yaml_functions.go pkg/merge/merge_deferred_test.go` — clean, no
  output.
- `atmos lint --changed` (patch-scoped `custom-gcl`) — 0 issues.
- Live re-verification against a locally rebuilt binary (`go build -o ./build/atmos .`): 20
  consecutive runs of `atmos describe component test-nested-function-collision -s test
  --process-templates=true --process-functions=true` from
  `tests/fixtures/scenarios/atmos-yaml-functions-merge/`, all 20 correct (previously ~40%
  failure rate over 35 pre-fix runs).

## Follow-ups

None.
