# Fix: `atmos vendor` `--component`/`--stack`/`--labels`/`--tags` selectors now compose as independent filters

**Date:** 2026-08-07

## Summary

A field-test pass of the `osterman/vendor-pull-stack-flag` PR (which added `--labels` and extended
`--tags`/`--stack` across all five `atmos vendor` subcommands) found two live bugs, several
consistency/coverage gaps, and — after user review pushed back on the initial fix's design — a
deeper architectural inconsistency: `--tags` was treated as a fourth mutually exclusive selector
"mode" everywhere, when it's actually an independent filter that should compose with `--component`
and `--stack`/`--labels` like any other filter. This fix:

1. `--tags` now composes with `--component` and `--stack`/`--labels` across all five subcommands
  (`pull`/`diff`/`clean`/`update`/`verify`), instead of being rejected as a conflicting selector.
  A component with no `vendor.yaml` entry (the common case for `--stack`, which installs via
  `component.yaml` regardless) has no declared tags and is naturally excluded by a non-empty
  `--tags` filter — the same way any filter excludes an entity missing the filtered attribute, not
  a special case.
2. `vendor pull --stack X --tags Y` and `vendor pull --component X --tags Y` (previously both
  silently accepted-but-ignored or flatly rejected, depending on the combination and how far this
  fix had progressed) now genuinely filter: the stack-resolved or named-component candidate set is
  narrowed by `vendor.yaml`-declared tags, erroring explicitly if that narrows it to nothing.
3. `vendor update --component X --tags Y` and `--stack`/`--labels`+`--tags` compose correctly (all
  already share a downstream AND-filter) and error explicitly when the combination matches
  nothing, instead of silently succeeding with an empty report.
4. `pull`'s and `update`'s "selector matched nothing" error wording/sentinel are now identical
  (both share a stack/labels-only selector vocabulary).
5. `clean`/`verify` now support `--type`/`--file`, matching `diff`/`update`.
6. Deduplicated two hand-maintained copies of the stack-walk logic in
  `internal/exec/vendor_component_utils.go`, and three hand-maintained copies of the
  component+tags matcher (`internal/exec`'s `shouldSkipSource`, `pkg/vendoring`'s
  `sourceMatchesFilter`, and `cmd/vendor/selector.go`'s tag-filtering) into one shared
  `vendoring.MatchesComponentTags` / `vendoring.FilterComponentsByDeclaredTags`.
7. Updated all five `website/docs/cli/commands/vendor/*.mdx` pages, which previously documented
  `--tags` as mutually exclusive with every other selector.

## Context

Two parallel research passes (Explore agents reading the actual implementation, docs, and tests)
confirmed two live bugs directly against source before any code changed: `internal/exec/vendor.go`'s
`validateVendorFlags` checked every flag pair except Stack×Tags, and `handleStackVendor` never reads
`flg.Tags` at all, so `pull --stack X --tags Y` silently ignored `--tags`. Separately,
`cmd/vendor/component_updater.go`'s `validateUpdateSelectorFlags` only guarded Component/Tags
against Stack/Labels, so `update --component X --tags Y` was accepted but could silently produce a
no-op empty report if the tag didn't match.

The fix went through two design iterations, both driven by user review:

**Iteration 1** treated both bugs as "reject the combination," matching how `diff`/`clean`/`verify`
already rejected `--stack`+`--tags` and `--component`+`--tags` via their shared
`vendorSelectorGroupCount`. User review caught that this was the wrong fix for `update`:
`--component` and `--tags` there both already operate on the same `vendor.yaml` `Sources[]` domain
via a pre-existing AND-filter (`sourceMatchesFilter`), so rejecting the combination removed working,
meaningful functionality ("update this specific component, but only if it's tagged X") instead of
fixing the actual bug (the silent-empty-report-on-mismatch UX gap). This iteration reverted the
`update` rejection, added an explicit zero-match error instead, and deduplicated `shouldSkipSource`/
`sourceMatchesFilter` into `vendoring.MatchesComponentTags` — but kept `pull --stack`+`--tags`
rejected, reasoning that `--stack` bypasses `vendor.yaml` for installation (using `component.yaml`
instead), so there was "no code today that could combine `--stack` and `--tags` meaningfully without
new cross-referencing logic."

**Iteration 2**: user review rejected that reasoning outright — "they are all filters; why don't
they work together? that's my complaint." This reframing dissolves the apparent complexity: a
component with no `vendor.yaml` entry simply has no tags to match, so it's excluded by a non-empty
`--tags` filter the same way any filter excludes a missing attribute — no special "domain crossing"
logic needed, just applying `--tags` as one more independent AND-predicate over whatever candidate
set `--component` or `--stack`/`--labels` already resolved. This is a materially different, more
consistent design than iteration 1's "keep tags mutually exclusive with stack, allow it with
component" compromise, and it applies uniformly across all five subcommands rather than being
`update`-specific. `vendoring.FilterComponentsByDeclaredTags` (built on the already-shared
`MatchesComponentTags`) implements this once and is reused everywhere a stack/labels- or
component-resolved candidate set needs `--tags` narrowing.

## Changes

- `pkg/vendoring/resolve.go`: added `FilterComponentsByDeclaredTags(vendorFile, components, tags)`
  — narrows an already-resolved component-name list by `vendor.yaml`-declared tags via
  `ListDeclaredSources` + `MatchesComponentTags`; a name with no declared entry is excluded by any
  non-empty tags filter.
- `cmd/vendor/selector.go`: `vendorSelectorGroupCount` now only tracks Component vs Stack/Labels
  exclusivity (dropped the `filterTags` parameter). `resolveVendorSelectorComponents` resolves a
  base selector (`--component` or `--stack`/`--labels`) then applies `--tags` as an independent
  narrowing filter via `FilterComponentsByDeclaredTags` — or, with no base selector, resolves
  directly off `vendor.yaml` as before. `errVendorSelectorsExclusive`'s message updated accordingly.
- `cmd/vendor/diff.go`, `clean.go`, `verify.go`: updated `vendorSelectorGroupCount` call arity;
  `diff.go`'s "no selector given" check now treats `--tags` alone as a valid selector.
- `cmd/vendor/component_updater.go`: `validateUpdateSelectorFlags` no longer rejects `--tags`
  combined with `--stack`/`--labels` (only Component×Stack/Labels remains rejected); doc comment
  explains `--tags` composes with everything via `pkg/vendoring/update.go`'s existing per-component
  filter, applied downstream regardless of how the candidate list was selected.
- `internal/exec/vendor.go`: removed `ErrValidateComponentFlag` (Component×Tags) and the
  iteration-1 `ErrValidateStackTagsFlag` (Stack×Tags) checks and their now-dead sentinels, along
  with `ErrValidateTagsLabelsFlag` (Tags×Labels). `handleVendorConfig` now returns an explicit
  "no components matched" error when `--component`+`--tags` is given but no `vendor.yaml` exists
  (the `component.yaml`-only path has no tags concept, so it can never match).
- `internal/exec/vendor_component_utils.go`: `handleStackVendor` now narrows its stack-resolved,
  type-grouped component set by `--tags` via new helpers `filterStackComponentsByTags` and
  `resolveAndFilterStackComponents` (the latter extracted to stay within this repo's
  cyclomatic-complexity/function-length lint budget), erroring explicitly if that empties an
  otherwise non-empty set. Also extracted `pullStackComponentsByType` (the sorted, per-type
  `ExecuteComponentVendorPullBatch` loop) for the same reason. Also carries the dedup from this
  fix's first pass: a shared `walkStackVendorComponents` helper used by both
  `resolveStackVendorComponents` (pull's `--stack` path) and `ResolveVendorComponentSelector`
  (update/diff/clean/verify's shared resolver) — previously two independently hand-rolled copies of
  the same stack walk. The zero-match branch uses `errUtils.ErrInvalidArgumentError` with the same
  wording `update` uses, replacing the old `errUtils.ErrStackNotFound`-based branch (confirmed via
  `grep` that no other vendor code path depends on that sentinel identity).
- `internal/exec/vendor_utils.go`: `shouldSkipSource` delegates to `vendoring.MatchesComponentTags`
  instead of its own `lo.Intersect`-based logic.
- `pkg/vendoring/update.go`: `MatchesComponentTags(src, component, tags)` (exported) extracted from
  `sourceMatchesFilter`, which now calls it before applying its own `componentType` check.
- `pkg/vendoring/updater/selection.go`: `UpdateSelectedComponents` returns an explicit
  `ErrInvalidArgumentError` ("No selected component matched the given --tags filter.") when an
  explicit component selection combined with `--tags` produces zero results.
- `website/docs/cli/commands/vendor/{vendor-pull,vendor-diff,vendor-clean,vendor-verify,vendor-update}.mdx`:
  replaced every "mutually exclusive with `--tags`" / "cannot be combined with `--tags`" statement
  with composition language, added composed-selector examples, and updated the flag-precedence note
  blocks (`vendor-pull.mdx`) describing how Atmos picks a vendoring manifest.
- Tests (all passing) span `internal/exec/vendor_test.go`, `vendor_component_utils_test.go`,
  `vendor_pull_sweep_test.go` (new `TestExecuteVendorPullCommand_StackAndTagsExcludesUntaggedComponents`,
  `TestExecuteVendorPullCommand_StackAndTagsMatchesDeclaredComponent`), `pkg/vendoring/resolve_test.go`
  (new `TestFilterComponentsByDeclaredTags_*`), `cmd/vendor/selector_test.go` (new
  `TestResolveVendorSelectorComponents_ComponentAndTagsCompose`/`_ExcludesMismatch`),
  `cmd/vendor/component_updater_test.go`, `cmd/vendor/vendor_test.go` (new
  `TestVendorUpdateCommand_ComponentAndTagsMismatchErrors`, `_StackAndTagsMismatchErrors`,
  `TestVendorDiffCommand_StackSelectorBatchMode`, `_LabelsSelector`, `_StackAndTagsCompose`,
  `_UnmatchedStackErrors`), `cmd/vendor/clean_test.go` (new `--stack`/`--labels`/`--type`/`--file`
  coverage plus `TestVendorCleanCmd_StackAndTagsCompose`), `cmd/vendor/verify_test.go` (same, plus
  `TestVendorVerifyCmd_StackAndTagsCompose` — `verify` previously had zero `--stack`/`--labels`
  coverage at any level), `pkg/tags/flags_test.go` (duplicate-key and whitespace-only-segment edge
  cases), `pkg/vendoring/update_test.go` (`TestUpdate_ComponentAndTagsFilterCompose`,
  `_ExcludesMismatch`), `pkg/vendoring/updater/selection_test.go`
  (`TestUpdateSelectedComponents_TagsMismatchErrors`, `_TagsMatchSucceeds`).
- `tests/test-cases/vendor-test.yaml`: added `atmos_vendor_pull_stack_and_labels`, a CLI-level
  regression case proving `--stack`+`--labels` compose end-to-end against the existing
  `tests/fixtures/scenarios/vendor-stack-labels/` fixture (previously only tested in isolation).

## Validation

- `go build ./...` and `go vet ./...` — clean, at every iteration.
- `atmos lint --changed` (patch-scoped, this repo's real CI gate) — `0 issues`. Several intermediate
  findings were caught and fixed across the pass: an `unparam` finding, a `lintroller` missing-
  `perf.Track` finding, two `godot` comment-formatting findings, and a `revive` cyclomatic-complexity
  finding (`handleStackVendor` exceeded both the complexity and function-length budgets once the
  `--tags` narrowing logic was added inline — resolved by extracting
  `resolveAndFilterStackComponents` and `pullStackComponentsByType`, per CLAUDE.md's mandatory
  refactoring guidance to extract named helpers and keep the caller a flat pipeline).
- `go test -count=1 ./cmd/vendor/... ./internal/exec/... ./pkg/vendoring/... ./pkg/tags/...` — all
  pass, including every new/updated test listed above, re-run after every design iteration.
- CLI-level fixture tests (`atmos_vendor_pull_stack`, `atmos_vendor_pull_labels`,
  `atmos_vendor_pull_stack_and_labels`) re-run with `./build/atmos` (a freshly built local binary
  explicitly prioritized on `PATH`, not the system-installed `/opt/homebrew/bin/atmos` the test
  harness picks up by default) to confirm they exercise this branch's actual code — all pass.
  Leftover generated fixture artifacts under `tests/fixtures/scenarios/vendor-stack-labels/` were
  removed after the run; none were gitignored but all were confirmed to be untracked test
  byproducts, not pre-existing files.
- `cd website && pnpm build` — succeeds after the `.mdx` doc updates (same pre-existing, unrelated
  warnings as before: a few broken-anchor and MDX-normalize notices on unrelated pages).
- A full `atmos test` (repo-wide) run hit one failure: `TestCIEnvironmentDetection`
  (`tests/cli_interactive_test.go`) hung on subprocess I/O. This is unrelated to any file touched by
  this fix (an interactive CI-detection test, not vendor/selector code) and did not reproduce when
  running the actually-affected packages directly — treated as a pre-existing sandbox/TTY
  environment issue, not a regression from this change.

## Follow-ups

None.
