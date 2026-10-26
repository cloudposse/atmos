# Fix: Preserve released include-query and data semantics

**Date:** 2026-10-17

## Summary

Restore four `!include` behaviors that regressed after v1.232.0: quoted yq
`"eval"` stays a string rather than enabling command execution, multi-result map
queries retain every entry, empty-string query results remain null, and ordinary
included data can carry unregistered YAML tags. All corrections apply to every
configuration edition.

## Context

Comparing v1.232.0 (`26edaff2cc`) with the merged PR candidate (`8870df9912`)
reproduced each failure twice, including unpinned projects and October 12/13 pins.
A query ending in `| "eval"` unexpectedly executed a nested `!exec` while describing
a component. A `.[]` query over `[{first: A}, {second: B}]` silently lost the second
entry, and `.missing // ""` changed a nullable-false Terraform variable from its
default to an empty string. Plain `!vault`/`!Custom` data started failing validation.

These are accidental regressions, not intentional edition transitions. Separate
CloudFormation short-form rewriting and numeric-string preservation remain intact.

## Changes

- Distinguish quoted yq strings and spaced `eval (.path)` calls from the unquoted
  include modifier, preserving pipelines and explicit evaluation.
- Normalize YAML collection presentation before querying, while preserving tags,
  scalar types, and quoting. The old decode/re-encode path used block collections;
  preserving flow collections produced adjacent `{...}` mappings that the decoder
  read only once. This was a presentation-style issue, not necessarily a stream of
  documents separated by `---`.
- Apply the general query evaluator's empty-string-to-null contract to YAML include
  queries, without changing empty values inside returned maps or unqueried data.
- Drop unregistered data tags when evaluation is off; retain registered foreign
  rewriters and migration diagnostics. Explicit evaluation still rejects unknown
  functions.
- Add parser, YAML loader, and component-description regression tests. The execution
  test verifies both the marker-file side effect and explicit opt-in execution using
  portable interpreter builtins, across old and unpinned editions.

## Validation

- Added tests before production changes; the initial run failed for all four reported
  reasons, including a marker file written by the quoted-eval case.
- New tests pass after the fixes, including duplicate-key rejection and controls for
  numeric strings, scalar streams, explicit eval, and foreign-tag rewriting.
- Full `pkg/function/parser`, `pkg/utils`, `pkg/config`, and
  `pkg/component/aws/cloudformation/manifest` suites pass.
- Focused `internal/exec` tests pass for include evaluation, workflow output retries,
  stack-name templates, and manifest component references.
- 47 paired CLI cases repeated twice against v1.232.0 (188 invocations) show no
  unexpected differences. The retained differences are the separately documented
  CloudFormation short-form rewrite and numeric-string correction.
- Eight real local Terraform 1.15.6 plans (two scenarios, both binaries, twice) retain
  both map entries and the nullable-false variable's default. No live infrastructure
  was used.

- Full PR patch lint passes with zero issues; the website build passes with existing
  MDX-normalization and dependency warnings.

## Follow-ups

The separately reported bulk workdir deployment/plan-verification failure and the
cross-subsystem compatibility implications of CloudFormation tag rewriting remain
outside these four include fixes.
