# Fix: `atmos vendor pull` no longer silently no-ops or drops flags on selector mismatches

**Date:** 2026-08-08

## Summary

A field-test pass of the `osterman/vendor-pull-stack-flag` branch (the day after
[2026-08-07's selector-composition fix](2026-08-07-vendor-selector-flag-consistency.md)) found four
more issues, all specific to `vendor pull`'s vendor.yaml-driven code path
(`internal/exec/vendor_utils.go`/`vendor.go`), which never shares the more robust selector resolver
`diff`/`clean`/`verify`/`update` already use (`cmd/vendor/selector.go`):

1. **Silent no-op (highest severity):** `atmos vendor pull -c vpc --tags compute`, where `vpc` is
   declared in `vendor.yaml` but its own tags don't include `compute` while some *other* declared
   component's tags do, exited 0 and installed nothing — no error, no warning.
   `validateTagsAndComponents` checked tag existence *globally* across every declared source
   instead of scoping the check to the named `--component`, so an out-of-scope component's tag
   could vouch for a mismatched one. This is the same silent-no-op bug class 2026-08-07's fix
   addressed for `vendor update --component --tags`, but that fix never reached `pull`.
2. **Misleading error ordering:** `-c <undeclared-component> --tags <tag-matching-nothing>`
   reported a generic "no components tagged with X" message instead of "component X is not
   defined," because the (global, now per-component) tags check ran before the component-existence
   check.
3. **Silent flag drop:** `atmos vendor pull -c vpc -c eks` silently kept only `eks` (pflag's
   last-occurrence-wins default for a plain string flag), dropping `vpc` with no error — surprising
   given `vendor update --component` is a repeatable slice that accumulates instead.
4. **Silent install-source divergence:** when a `--stack`/`--labels`-resolved component has *both*
   its own `component.yaml` and a `vendor.yaml` entry, `atmos vendor pull -c <name>` and
   `atmos vendor pull --stack ...` install from different sources for the identical target
   directory, with no indication either way. This is documented, intentional precedence (see
   `vendor-pull.mdx`'s selection-precedence note) — `--stack`/`--labels` always install from
   `component.yaml` "regardless of vendor.yaml" — so this fix does not change behavior, it only
   warns when the divergence risk exists.

## Context

This surfaced from a `field-test` skill pass specifically targeting the flag-composition work from
2026-08-07/086efe1788, using a real fixture (`vendor.yaml` with tagged sources + a stack with
`metadata.labels`) and the actually-built `atmos` binary, not just unit tests. Comparing `vendor
pull -c vpc --tags compute` against the equivalent `vendor diff -c vpc --tags compute` (which
correctly errors) confirmed the bug was isolated to `pull`'s own resolution path
(`ExecuteAtmosVendorInternal`/`validateTagsAndComponents`/`processAtmosVendorSource`), not a design
gap shared with the other four subcommands.

The user's review of finding 4 asked specifically whether a "merge tags, then filter" fix could
leak out-of-scope tags into the check — which is exactly finding 1's root cause: the tags-existence
check must be scoped to the selector-already-narrowed candidate set (here, the single named
`--component`), never a global union across `vendor.yaml`. The corrected scoping mirrors what
`cmd/vendor/selector.go`'s `resolveVendorSelectorComponents` + `pkg/vendoring/resolve.go`'s
`FilterComponentsByDeclaredTags` already do correctly for the other four subcommands — this fix
brings `pull`'s bespoke path in line with that existing, already-correct pattern rather than
inventing a new one.

## Changes

- `internal/exec/vendor_utils.go`: `validateTagsAndComponents` now checks component existence
  first (fixes #2), then, when `--component` is set, scopes the tags check to that component's own
  declared `Tags` only (fixes #1) — returning `errUtils.ErrInvalidArgumentError` with a hint
  naming the mismatched component/tags, matching the wording `diff`/`clean`/`verify`/`update`
  already use for "selector matched nothing." The bare-`--tags`-only path (no `--component`) keeps
  its existing global-union check and `ErrNoComponentsWithTags` wording, unchanged.
- `cmd/vendor/vendor.go`: `vendor pull`'s `--component`/`-c` flag is now a string slice
  (`WithStringSliceFlag`, matching `vendor update`'s registration) instead of a plain string.
  `internal/exec/vendor.go`'s pre-existing `parseVendorComponentFlag`/`ErrSingleComponentRequired`
  (previously reachable only via `vendor update --pull`'s delegation) already handled this slice
  case correctly — no other code changes were needed to make repeated `-c` reject with a clear
  error instead of silently keeping the last value (fixes #3).
- `internal/exec/vendor_component_utils.go`: `handleStackVendor` now calls a new
  `warnAboutVendorYamlShadowing` after resolving (and tags-filtering) the stack-scoped component
  set, emitting one `ui.Warningf` per resolved component that also has a `vendor.yaml` entry. This
  is best-effort and never blocks or errors the pull: a missing or malformed `vendor.yaml` is
  silently ignored here, since `--stack`/`--labels` are designed to work with no `vendor.yaml` at
  all when `--tags` isn't also forcing one to be read (fixes #4).

## Validation

- Added regression tests reproducing each bug against the actual buggy code first, confirmed they
  failed, then confirmed they pass after each fix:
  - `internal/exec/vendor_utils_test.go`'s `TestValidateTagsAndComponents` (3 new subtests) and
    `internal/exec/vendor_pull_sweep_test.go`'s new
    `TestExecuteVendorPullCommand_ComponentAndTagsMismatch_WithVendorFile_ReturnsInvalidArgument`
    (end-to-end, real `ExecuteVendorPullCommand`) for #1/#2.
  - `cmd/vendor/vendor_test.go`'s new `TestVendorPullCmd_RepeatedComponentFlagErrors` (real
    `vendorPullCmd` flags via `vendorPullParser`) for #3.
  - `internal/exec/vendor_component_utils_test.go`'s new
    `TestHandleStackVendor_ComponentAlsoInVendorYaml_WarnsAboutDivergentSource` for #4.
- `go build ./...` clean.
- `go test ./internal/exec/... -run Vendor -count=1` and `go test ./cmd/vendor/... -count=1`: all
  pass, including every pre-existing test (no regressions from the `--component` flag-type change
  or the `validateTagsAndComponents` restructure).
- `atmos fix lint` (patch-scoped `--new-from-rev=origin/main`): no new findings from this fix's own
  changes; the run surfaced two pre-existing findings from earlier commits on this branch
  (`dupl` in `internal/exec/vendor_component_utils_test.go`, `godot` in `cmd/vendor/diff_test.go`)
  that predate this fix and are not addressed here.

## Follow-ups

None.
