# Fix: `describe component --provenance` no longer drops populated aws/cloudformation sections

**Date:** 2026-09-09

## Summary

`atmos describe component <name> -s <stack>` for an `aws/cloudformation` component never showed
`path`, `stack_name`, `parameters`, `template`, `termination_protection`, `capabilities`, `tags`,
`stack_policy`, `role_arn`, `notification_arns`, `disable_rollback`, or `timeout_in_minutes` — even
though these are real, documented top-level fields and the same values were correctly read and used
at real `apply`/`delete` execution time. The bug was in the provenance-rendering pipeline
(`pkg/provenance/data_transform.go`), not in component-type population or in
`FilterComputedFields`'s allowlist, both of which were already correct.

## Context

The task hypothesis was that CFN-specific top-level sections never made it into
`configAndStacksInfo.ComponentSection` in the first place — i.e. that `tryProcessWithComponentType`
(`internal/exec/describe_component.go`) or the stack-processing pipeline it delegates to had a
missing per-component-type whitelist entry for CFN, mirroring the known "new component TYPE
describe/list whitelist" pattern in this codebase (see `.claude/skills/atmos-core-component-development`).

That hypothesis did not hold up under direct investigation:

- `internal/exec/stack_processor_process_stacks_helpers_extraction.go:420-445` already defines
  `cloudFormationComponentSectionKeys` and `extractCloudFormationComponentSection`, populated
  whenever `opts.ComponentType == cfg.CloudFormationComponentType`
  (`stack_processor_process_stacks_helpers_extraction.go:328-330`).
- `internal/exec/stack_processor_merge.go:719-733` deep-merges `result.BaseComponentCloudFormation`
  and `result.ComponentCloudFormation` into the final component map (`comp`) for CFN components,
  mirroring the equivalent Helm block immediately above it.
- A direct call to `ExecuteDescribeComponentWithContext` for the `examples/cloudformation` `demo`
  component (writing a throwaway test to dump `result.ComponentSection` keys, then
  `FilterComputedFields(result.ComponentSection)` keys) showed **all** of `path`, `stack_name`,
  `parameters`, `hooks`, `metadata`, `provision`, `settings`, and `component` present and correct,
  identically whether `atmosConfig.TrackProvenance` was `true` or `false`.
- Running the CLI directly with `--provenance=false` also showed every field correctly. Running the
  exact same command with **no flag** (i.e. the actual default, since `--provenance` defaults to "on"
  per `describe.provenance` in `atmosConfig`) reproduced the bug — and dropped not just the CFN
  fields but *every* top-level section except `vars`/`import`/`flags`, even for a `terraform`
  component with a `settings`/`hooks` block. That proved the drop was generic to the provenance
  render path, not CFN-specific, and not a population-side gap at all.

The actual root cause: `pkg/provenance/data_transform.go`'s `filterEmptySections` (called from
`prepareYAMLForProvenance`, `pkg/provenance/tree_renderer.go:187`, on the way to
`RenderInlineProvenanceWithStackFile`) kept a top-level section **only if `hasSectionProvenance`
found a recorded provenance entry for it** (`data_transform.go:82-86`, pre-fix). Its own doc comment
said the intent was narrower — "removes top-level sections that have no provenance... prevents
displaying sections like `backend: {}` or `overrides: {}`" (i.e. hide *empty*, auto-generated
placeholder sections) — but the implementation conflated "no provenance was ever recorded for this
key" with "this key is empty," and dropped the section outright in both cases.

Most nested sections *do* get per-key provenance recorded, because their final value comes from
`pkg/merge.MergeWithOptionsAndContext` (`pkg/merge/merge.go:634-701`), which calls
`MergeWithProvenance` → `recordProvenanceRecursive` whenever provenance tracking is enabled. But
`aws/cloudformation`'s `path`/`stack_name`/`parameters`/`hooks`/`settings`/`provision`/etc. are
copied into the final component map as **plain values** via `extractCloudFormationComponentSection`
and the direct-assignment loop in `stack_processor_merge.go:730-732`
(`for key, value := range finalComponentCloudFormation { comp[key] = value }`) — never merged
key-by-key through the provenance-tracked path — so no entry is ever recorded for them, and
`filterEmptySections` treated "never recorded" as "must be empty" and silently dropped real,
non-empty data. (The renderer itself, `RenderInlineProvenanceWithStackFile`/
`processYAMLLineWithProvenance` in `pkg/provenance/tree_renderer.go:288-314`, was confirmed
*not* to drop unannotated lines — it just omits the inline `# ● [1] file:line` comment for them —
ruling out the render loop as the culprit and pointing squarely at the upstream filter.)

`FilterComputedFields`'s allowlist (`internal/exec/describe_component.go:637-705`) was already
correct and complete for CFN, as the task's own investigation found — it was simply unreachable
because `filterEmptySections` ran afterward, deeper in the `--provenance` render path, and removed
the sections before they ever reached the terminal.

## Changes

- `pkg/provenance/data_transform.go:68-123` — `filterEmptySections` now keeps a section if it has
  recorded provenance **or** its value is genuinely non-empty (`isEmptyValue`: `nil`, an empty map,
  or an empty slice). Only sections that are *both* unprovenanced *and* empty are dropped now,
  matching the function's original stated intent instead of its previous, broader implementation.
  Scalars (including the empty string) are never treated as empty, since a deliberately-set empty
  string is real configured data, not a generated placeholder.
- `pkg/provenance/provenance_helpers_test.go` — updated
  `TestRenderInlineProvenance_YAMLMarshallingError`. That test previously asserted the render
  produced `{}` for a map of `func`/`chan` values; it worked only because the old, over-broad filter
  happened to remove those unprovenanced keys before they ever reached the YAML encoder. With the
  filter now preserving non-empty values, the encoder legitimately reports a marshal error for the
  unmarshalable types, which `RenderInlineProvenanceWithStackFile` already surfaces as a graceful
  error string (no panic) — the test now asserts that message instead.
- `pkg/provenance/data_transform_test.go` — added
  `TestFilterEmptySectionsKeepsNonEmptyUnprovenancedSections`, reproducing the exact CFN shape
  (`path`, `stack_name`, `parameters`, `termination_protection`, `capabilities`, `settings`, `hooks`
  all unprovenanced but non-empty, alongside genuinely empty/unprovenanced `backend`/`overrides`
  placeholders) and asserting the former survive while the latter are still dropped.
- `internal/exec/describe_component_provenance_test.go` — added
  `TestDescribeComponent_CloudFormationProvenanceKeepsUnprovenancedFields`, an end-to-end regression
  test against `examples/cloudformation`'s `demo` component: calls
  `ExecuteDescribeComponentWithContext` with `TrackProvenance = true`, applies
  `FilterComputedFields` (the CLI's default schema filter), and renders through
  `p.RenderInlineProvenanceWithStackFile` exactly as `ExecuteDescribeComponentCmd` does — then
  asserts `path`, `stack_name`, `parameters`, `hooks`, `settings`, `provision`, and `metadata` all
  appear in the rendered output. Verified this test fails against the pre-fix
  `data_transform.go` (via `git stash` of just that file) and passes with the fix restored.
  **This specific test lives in the `aws/cloudformation` PR stack (not yet merged to `main`), since
  it depends on `examples/cloudformation` and `cfg.CloudFormationComponentType`; the actual bug fix
  below (`pkg/provenance/data_transform.go`) is general and applies to every component type — it
  ships here, on `main`, backed by the general-purpose
  `TestFilterEmptySectionsKeepsNonEmptyUnprovenancedSections` unit test instead.**

No changes were made to `internal/exec/describe_component.go`'s `FilterComputedFields`,
`detectComponentType`, or `tryProcessWithComponentType` — all three were already correct, and the
task's original hypothesis about a missing CFN whitelist entry there was disproven by direct testing.

## Validation

- `go build ./...` — passes.
- `go test ./pkg/provenance/...` — passes, including the new and updated tests.
- `go test ./internal/exec/...` (and targeted `-run` subsets covering `Describe|Provenance|CloudFormation`)
  — passes.
- `go test ./pkg/merge/...` — passes (unaffected).
- Manual CLI check: `atmos build` from repo root, then from `examples/cloudformation`:
  `atmos describe component demo -s local` (no flags — the actual default path, since
  `--provenance` defaults to enabled) now prints `path: template.yaml`, `stack_name:
  atmos-cfn-demo-local`, `parameters:`, `hooks:`, `settings:`, `provision:`, and `metadata:` — all
  previously silently dropped from this exact invocation. (A later fix on this branch excludes the
  synthetic, always-injected `component` key from this same resurrection logic, so `component:`
  does *not* appear here unless the stack YAML explicitly sets it — see the `git log` history on
  `pkg/provenance/data_transform.go` for that follow-up.)
- `GOTOOLCHAIN=go1.26.6 ./custom-gcl run --config=.golangci.yml --allow-serial-runners
  --new-from-rev=origin/main` — reports findings only in files owned by other concurrent fixes on
  this branch (`pkg/component/aws/cloudformation/*`, `pkg/utils/component_path_utils.go`,
  `internal/exec/describe_affected_components_test.go`, `internal/exec/yaml_func_terraform_output.go`);
  zero findings on any file this fix touched.
- `atmos test` (full suite) was run once; `github.com/cloudposse/atmos/tests` failed with an HTTP2
  transport read timeout inside `go test`'s own goroutine dump (a real-network dependency stalling
  in this sandboxed environment), unrelated to `pkg/provenance` or `internal/exec` — those two
  packages' own suites passed cleanly on their own, and the stalled package's failure carries no
  stack frame anywhere near this change.

## Follow-ups

None. The fix is scoped to the render-time filter and does not require plumbing per-key provenance
recording into the CFN (or Helm) "single bag" extraction path — doing so would be a larger,
separate enhancement (recording provenance for `path`/`stack_name`/etc. individually) with no
functional bug attached; today's fix already restores correct, complete `describe component` output
for these fields, only without a per-key `# ● [1] file:line` annotation on them specifically.
