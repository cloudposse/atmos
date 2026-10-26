# Fix: Second field-test pass on `config set`/`stack set` auto type inference

**Date:** 2026-08-07

## Summary

A second hands-on field-test pass (the first landed in commit `d751acb693`) against the
`osterman/field-test-config-commands` branch's auto type-inference feature for `atmos config set`,
`atmos stack set`, and `atmos vendor config set` found three new high-severity bugs — two of them
silent data mutation — plus a misleading error message and an incorrect fixture comment. All were
reproduced live against a real binary before being fixed.

## Context

The branch's core feature commit (`8dc615c578`) made `--type` default to `auto` for `config set`
and `stack set`, inferring a scalar type from the Atmos config schema or the existing value at the
target path. Running the actual CLI (not just unit tests) against the branch's own
`tests/fixtures/scenarios/config-field-test` fixture surfaced three problems the schema/unit-test
coverage didn't catch:

- `pkg/yaml.GetType`'s tag switch fell through to `(TypeString, true)` for a `!!seq`/`!!map`
  existing value, which callers read as "confidently inferred string" rather than "ambiguous." This
  let `stack set vars.taglist x -s nonprod -c mycomponent` (no `--type`) silently overwrite an
  existing list with the plain string `x`, printing a success message with no warning. Same for
  maps.
- `pkg/yaml.buildValidatedRHS` wrote the user's raw literal verbatim/unquoted for `--type=int`.
  `strconv.ParseInt("010", 10, 64)` accepts a leading-zero string as decimal 10, but `yaml.v3`'s
  default resolver re-parses an unquoted `010` as octal (8) on the next read — confirmed through
  the full `describe component` resolution pipeline, i.e. this would reach a real Terraform
  varfile as the wrong number.
- `stack set`/`stack delete` resolving via provenance to a shared imported catalog file silently
  mutated that file, changing the effective value for every other stack/component that imports it
  — confirmed `-s nonprod ... delete vars.baz` (a catalog-only value) also removed it for `-s prod`,
  with no warning.

A fourth issue (`--type=int` on a value that overflows int64 reporting `"is not an integer"`,
which is misleading — it is an integer, just out of range) and a fixture-comment inaccuracy
(`settings.fragment_only_setting` was claimed to be visible via the merged `atmos describe config`
view but was actually silently dropped everywhere, since atmos.yaml's root `settings:` decodes
into the strongly-typed `AtmosSettings` struct with no catch-all field) were fixed alongside these.

An attempted fix for a fifth, lower-severity finding (bare `nan`/`inf` accepted by
`strconv.ParseFloat` but not valid YAML, which needs `.nan`/`.inf`) initially tried to normalize
the written value to the dotted YAML 1.1 spelling. That introduced a regression: `.nan` as a raw
`SetRaw` RHS is evaluated as a yq expression, and a leading `.` is yq's path-navigation operator —
`.nan` silently resolved to "look up field `nan` on the root document," writing `null` instead of
NaN. There is no verified-safe way to insert these two values through the current `SetRaw`
mechanism, so the fix instead rejects them with an actionable error rather than writing the wrong
value.

## Changes

- `pkg/yaml/edit.go`: `GetType` now returns `(TypeYAML, true)` for `!!seq`/`!!map` tags instead of
  `(TypeString, true)`, so callers can distinguish "existing value is non-scalar" from "existing
  value really is a string."
- `pkg/yaml/errors.go`: added `ErrTypeInferenceNonScalar`.
- `pkg/yaml/typed.go`: `buildValidatedRHS` now writes the canonical decimal form for `TypeInt`
  (`strconv.FormatInt`, closing the leading-zero/octal gap) and a canonical form for `TypeFloat`
  via the new `formatYAMLFloat` helper (ensures whole numbers keep the `!!float` tag); distinguishes
  `strconv.ErrRange` from a syntax error for a clearer out-of-range message; explicitly rejects
  NaN/Infinity for `TypeFloat` instead of attempting to write `.nan`/`.inf`.
- `cmd/config/operations.go`, `cmd/stack/operations.go`: `effectiveValueType`/
  `effectiveStackValueType` now return an error (`ErrTypeInferenceNonScalar`, with a hint to pass
  `--type=yaml` or `--type=string`) when auto-inference lands on `TypeYAML`, instead of silently
  falling through to a string coercion.
- `cmd/stack/operations.go`: added `editTarget.sharedFile` (computed via the new
  `isTopLevelStackFile`, using `atmosConfig.StackConfigFilesAbsolutePaths`) and `warnIfSharedFile`,
  called from `runStackSet`/`runStackDelete` before mutating a file reached only through `import:`.
- `cmd/vendor/config.go`: `--type` flag help text now mentions `auto` is a recognized-but-rejected
  keyword.
- `website/docs/cli/commands/vendor/config/set.mdx`: documented the same.
- `tests/fixtures/scenarios/config-field-test/atmos.d/extra.yaml`,
  `tests/fixtures/scenarios/config-field-test/atmos.yaml`: replaced the arbitrary
  `settings.fragment_only_setting` key (silently dropped by the typed `AtmosSettings` struct, so
  invisible everywhere, not just to `config get/set`) with the real `settings.list_merge_strategy`
  field, which actually demonstrates the atmos.d-vs-root visibility gap the fixture was built to
  test; corrected both files' comments to match.
- Added regression tests: `pkg/yaml/edit_test.go` (`TestGetType_ListAndMap`),
  `pkg/yaml/typed_test.go` (int/float canonicalization, overflow message, NaN/Inf rejection),
  `cmd/config/operations_test.go` (`TestConfigSetCommand_AutoRejectsExistingList`),
  `cmd/stack/operations_test.go` (list/map rejection, the previously-untested
  `mycomponent-anchor-owner`/`mycomponent-anchor-user` fixture components exercised through the
  real CLI command layer for the first time, and `editTarget.sharedFile` computation).

## Validation

- `go build ./...` — clean.
- `go test ./pkg/yaml/... ./cmd/config/... ./cmd/stack/... ./cmd/vendor/... ./pkg/config/... ./pkg/ai/tools/atmos/...`
  — all pass, including 12 new regression tests, with no changes to pre-existing tests.
- Live-drove every fix against a real `./build/atmos` binary run against a scratch copy of
  `tests/fixtures/scenarios/config-field-test`: list/map clobber now rejected with the value intact
  on disk; `--type=int 010` now round-trips as `10` (confirmed via `stack get` and
  `describe component`), not `8`; overflow now reports "out of range"; the shared-catalog-file
  warning prints before the mutation; the corrected fixture's `settings.list_merge_strategy` is
  visible via `atmos describe config -q .settings.list_merge_strategy` and correctly not found via
  `atmos config get`.
- `atmos fix lint` (patch-scoped, `--new-from-rev=origin/main`) — 0 issues, after fixing 3 `godot`
  findings in new comments.

## Follow-ups

None.
