# Fix: `atmos vendor` `--stack`/`--tags`/`--labels` selector validation made consistent across subcommands

**Date:** 2026-08-07

## Summary

A field-test pass of the `osterman/vendor-pull-stack-flag` PR (which added `--labels` and extended
`--tags`/`--stack` across all five `atmos vendor` subcommands) found two live bugs and several
consistency/coverage gaps. This fix:

1. `vendor pull --stack X --tags Y` now rejects the combination instead of silently dropping
   `--tags` and vendoring every component in the stack.
2. `vendor update --component X --tags Y` composes correctly (both already filter the same
   `vendor.yaml` `Sources[]` domain) and now errors explicitly when the combination matches
   nothing, instead of silently succeeding with an empty report.
3. `pull`'s and `update`'s "selector matched nothing" error wording/sentinel are now identical
   (both share a stack/labels-only selector vocabulary).
4. `clean`/`verify` now support `--type`/`--file`, matching `diff`/`update`.
5. Deduplicated two hand-maintained copies of the stack-walk logic in
   `internal/exec/vendor_component_utils.go`, and two hand-maintained copies of the
   component+tags matcher (`internal/exec`'s `shouldSkipSource` / `pkg/vendoring`'s
   `sourceMatchesFilter`) into one shared `vendoring.MatchesComponentTags`.

## Context

Two parallel research passes (Explore agents reading the actual implementation, docs, and tests)
confirmed the two headline bugs directly against source before any code changed: `internal/exec/
vendor.go`'s `validateVendorFlags` checked every flag pair except Stack×Tags, and
`handleStackVendor` never reads `flg.Tags` at all, so `pull --stack X --tags Y` silently ignored
`--tags`. Separately, `cmd/vendor/component_updater.go`'s `validateUpdateSelectorFlags` only guarded
Component/Tags against Stack/Labels, so `update --component X --tags Y` was accepted but could
silently produce a no-op empty report if the tag didn't match.

The initial pass treated both as "reject the combination," matching how `diff`/`clean`/`verify`
already reject `--stack`+`--tags` and `--component`+`--tags` via their shared `vendorSelectorGroupCount`.
User review caught that this was the wrong fix for the `update` case: unlike `--stack`/`--labels`
(which for `pull` bypasses `vendor.yaml` entirely and resolves via `component.yaml`, a domain with no
`tags` concept at all — so there's no code today that could combine `--stack` and `--tags`
meaningfully without new cross-referencing logic), `--component` and `--tags` on `update` both
already operate on the same `vendor.yaml` `Sources[]` domain via a pre-existing AND-filter
(`sourceMatchesFilter`). Rejecting that combination removed working, meaningful functionality
("update this specific component, but only if it's tagged X") instead of fixing the actual bug,
which was purely the silent-empty-report-on-mismatch UX gap. The fix was revised to: keep
`pull --stack`+`--tags` rejected (the domains genuinely don't compose today), but revert the
`update --component`+`--tags` rejection and instead make a real mismatch surface as an explicit
error — while also deduplicating the two independently-hand-maintained component+tags matcher
functions the review surfaced (`internal/exec`'s `shouldSkipSource` and `pkg/vendoring`'s
`sourceMatchesFilter`), since fixing the bug once in a shared function is more robust than patching
each copy separately.

Whether `pull --stack`+`--tags` (and `--labels`+`--tags` more broadly) should eventually support a
real intersection (stack-resolved component names filtered by `vendor.yaml`-declared tags) is an
open design question raised during review and not yet decided — no work has been deferred on it, so
it is not tracked as a follow-up here.

## Changes

- `internal/exec/vendor.go`: added `ErrValidateStackTagsFlag` sentinel; `validateVendorFlags` now
  rejects `Stack != "" && len(Tags) > 0`.
- `internal/exec/vendor_component_utils.go`: `handleStackVendor`'s zero-match branch now returns
  `errUtils.ErrInvalidArgumentError` with the same wording `update` uses ("No components matched
  the given --stack/--labels selector."), replacing the old `errUtils.ErrStackNotFound`-based
  branch (confirmed via `grep` that no other vendor code path depends on that sentinel identity).
  Extracted a shared `walkStackVendorComponents` helper, used by both `resolveStackVendorComponents`
  (pull's `--stack` path, further filtered to components with a `component.yaml`) and
  `ResolveVendorComponentSelector` (update/diff/clean/verify's shared resolver, further flattened
  and deduped by name alone) — previously two independently hand-rolled copies of the same walk.
- `internal/exec/vendor_utils.go`: `shouldSkipSource` now delegates to the new
  `vendoring.MatchesComponentTags` instead of its own `lo.Intersect`-based logic.
- `pkg/vendoring/update.go`: extracted `MatchesComponentTags(src, component, tags)` (exported) from
  `sourceMatchesFilter`, which now calls it before applying its own `componentType` check.
- `pkg/vendoring/updater/selection.go`: `UpdateSelectedComponents` now returns an explicit
  `ErrInvalidArgumentError` ("No selected component matched the given --tags filter.") when an
  explicit component selection combined with `--tags` produces zero results, instead of returning
  an empty report with no error.
- `cmd/vendor/component_updater.go`: `validateUpdateSelectorFlags` no longer rejects
  `--component`+`--tags` outside the `--stack`/`--labels` gate (reverted from the initial, incorrect
  fix); doc comment updated to explain why the combination is valid.
- `cmd/vendor/clean.go`, `cmd/vendor/verify.go`: registered `--type`/`--file` flags and threaded
  `ComponentType`/`TypeChanged`/`VendorFile` into their `resolveVendorSelectorComponents` calls,
  matching `diff.go`.
- Tests (all passing): `internal/exec/vendor_test.go`, `internal/exec/vendor_component_utils_test.go`,
  `internal/exec/vendor_pull_sweep_test.go` (new `TestExecuteVendorPullCommand_StackAndTagsRejected`
  end-to-end regression), `cmd/vendor/component_updater_test.go`, `cmd/vendor/vendor_test.go` (new
  `TestVendorUpdateCommand_ComponentAndTagsMismatchErrors`, `TestVendorDiffCommand_StackSelectorBatchMode`,
  `TestVendorDiffCommand_LabelsSelector`, `TestVendorDiffCommand_UnmatchedStackErrors`),
  `cmd/vendor/clean_test.go` (new `--stack`/`--labels`/`--type`/`--file` coverage),
  `cmd/vendor/verify_test.go` (same, `verify` previously had zero `--stack`/`--labels` coverage at
  any level), `pkg/tags/flags_test.go` (duplicate-key and whitespace-only-segment edge cases),
  `pkg/vendoring/update_test.go` (`TestUpdate_ComponentAndTagsFilterCompose`,
  `TestUpdate_ComponentAndTagsFilterExcludesMismatch`), `pkg/vendoring/updater/selection_test.go`
  (`TestUpdateSelectedComponents_TagsMismatchErrors`, `TestUpdateSelectedComponents_TagsMatchSucceeds`).
- `tests/test-cases/vendor-test.yaml`: added `atmos_vendor_pull_stack_and_labels`, a CLI-level
  regression case proving `--stack`+`--labels` compose end-to-end against the existing
  `tests/fixtures/scenarios/vendor-stack-labels/` fixture (previously only tested in isolation).

## Validation

- `go build ./...` and `go vet ./...` — clean.
- `atmos lint --changed` (patch-scoped, this repo's real CI gate) — `0 issues` (one intermediate
  `unparam` finding and one `lintroller` missing-`perf.Track` finding were caught and fixed during
  the pass).
- `go test ./cmd/vendor/... ./internal/exec/... ./pkg/vendoring/... ./pkg/tags/...` — all pass,
  including every new/updated test listed above.
- CLI-level fixture tests (`atmos_vendor_pull_stack`, `atmos_vendor_pull_labels`,
  `atmos_vendor_pull_stack_and_labels`) re-run with `./build/atmos` (a freshly built local binary
  explicitly prioritized on `PATH`, not the system-installed `/opt/homebrew/bin/atmos` the test
  harness picks up by default) to confirm they exercise this branch's actual code — all pass.
  Leftover generated fixture artifacts (`main.tf`/`README.md`/`vendor.lock.yaml` under
  `tests/fixtures/scenarios/vendor-stack-labels/`) were removed after the run; none were gitignored
  but all were confirmed to be untracked test byproducts, not pre-existing files.
- A full `atmos test` (repo-wide) run hit one failure: `TestCIEnvironmentDetection`
  (`tests/cli_interactive_test.go`) hung on subprocess I/O. This is unrelated to any file touched by
  this fix (an interactive CI-detection test, not vendor/selector code) and did not reproduce when
  running the actually-affected packages directly — treated as a pre-existing sandbox/TTY
  environment issue, not a regression from this change.

## Follow-ups

None.
