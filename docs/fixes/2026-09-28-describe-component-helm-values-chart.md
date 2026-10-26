# Fix: `describe component` now surfaces native Helm `chart` and `values`

**Date:** 2026-09-28

## Summary

`atmos describe component` (with the default `describe.component.filter: schema`) dropped the native
Helm `chart`, `values`, and `values_files` sections. For a native Helm component those are the
primary configuration a user wants to inspect (they drive the Helm release), yet they were hidden
from the default output. `atmos describe stacks` does not apply this filter, so the same component
showed them there - a surprising inconsistency. This change adds `chart`, `values`, and
`values_files` to the schema-filter allowlist so the default `describe component` output includes
them. Fixes cloudposse/atmos#3218.

## Context

`FilterComputedFields` (`internal/exec/describe_component.go`) scopes the default `describe component`
output. It runs only when the filter is the default `schema` (`describe.component.filter`); `full`
restores every computed field, and any `--query` runs against the full, unfiltered section.

The function uses an intentional **allowlist** (`fieldsToKeep`) of the sections a stack manifest can
define. That allowlist did not include the native Helm sections, so `chart`, `values`, and
`values_files` were filtered out of the default output even though:

- they are the primary configuration for a native Helm component, and
- they are already defined in the manifest JSON schema (`helm_component_manifest` in
  `pkg/datafetcher/schema/atmos/manifest/1.0.json` and the two mirror copies), i.e. they are valid,
  documented stack config - the gap was purely in the describe filter, not the schema.

## Changes

- `internal/exec/describe_component.go`:
  - Added `chart`, `values`, and `values_files` to the `fieldsToKeep` allowlist in
    `FilterComputedFields`.
  - Updated the allowlist comment to record this incremental addition (alongside the earlier
    `flags` addition from PR #2992) and to note that the broader question - whether to surface
    additional sections and whether this filter should be driven by the manifest schema instead of
    a hand-maintained list - is tracked separately (see #3223).
- `internal/exec/describe_component_test.go`:
  - `TestFilterComputedFields` - added a case asserting `chart`/`values`/`values_files` are kept
    while Atmos-computed bookkeeping is still removed.
  - `TestDescribeComponentSchemaFilterKeepsHelmValuesAndChart` - an offline end-to-end test that
    runs the real describe pipeline against the `examples/helm` fixture and verifies the default
    schema filter keeps `chart`/`values` while dropping computed fields.

## Scope

This is deliberately the minimal fix for the visible #3218 gap. It keeps the existing intentional
allowlist rather than replacing it. The larger design discussion - a schema-driven filter, whether
computed-but-useful fields like `workspace`/`component_type` belong in default output, and the
`additionalProperties` inconsistency between component types in the manifest schema - is out of
scope here and tracked separately (#3223).

## Validation

```bash
# Unit + offline end-to-end tests.
go test ./internal/exec/ -run 'TestFilterComputedFields|TestDescribeComponentFilter|TestDescribeComponentSchemaFilterKeepsHelmValuesAndChart' -count=1

# Native Helm component (examples/helm): the default schema filter now surfaces chart/values.
atmos describe component demo -s dev --format json | jq 'keys'   # includes "chart" and "values"

# No CLI golden snapshots change (no native-Helm describe-component snapshots exist).
go build ./...
atmos lint --changed
```
