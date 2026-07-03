# Respect metadata.enabled when evaluating components in the describe pipeline

## Summary

`atmos describe affected` (and the shared `describe stacks` / `list` pipeline) evaluated the
`!terraform.state` / `!terraform.output` YAML functions and the `atmos.Component(...)` template
function for **every** component, regardless of `metadata.enabled`. A component disabled via
`metadata.enabled: false` that references an unprovisioned component's state therefore caused a hard
`terraform state not provisioned` failure, even though the disabled component is (correctly) excluded
from the final affected list. See cloudposse/atmos#2654.

## Root cause

`describe affected` describes the current and base stacks via `ExecuteDescribeStacksWithAuthDisabled`,
which runs the shared `describeStacksProcessor`. `processComponentEntry` processed templates and YAML
functions for each component with no `metadata.enabled` gate. The enabled-aware filters
(`shouldSkipComponent`, `FilterAbstractComponents`) only run when assembling the affected list — after
the describe phase has already evaluated (and failed on) the disabled component.

## Fix

Gate live, state-reading evaluation on `metadata.enabled`, using the existing `isComponentEnabled`
helper (which reads only `metadata.enabled`, never `vars.enabled`):

- **YAML functions (`!terraform.state` / `!terraform.output`):** in `processComponentEntry`, when the
  component is disabled, append the terraform state/output functions to a clone of the per-pass skip
  list (`disabledComponentTerraformSkip`). A skipped function falls through unchanged, so the raw
  `!terraform.*` string is preserved and no backend read occurs.
- **Template function (`atmos.Component(...)`):** in `componentFunc`, when the *enclosing* component is
  disabled (`enclosingComponentDisabled`, read from `info.ComponentSection`), return
  `emptyComponentSections()` — the standard sections as empty maps, including an empty `outputs` — with
  no describe and no state read. Empty maps keep `(atmos.Component …).outputs.x` / `.vars.x` nil-safe
  instead of erroring.

A nil info or absent metadata is treated as enabled, so non-describe template contexts (e.g.
datasource templates built with an empty info) are never affected.

## Scope and behavior

The fix lives in the shared processor and `componentFunc`, so it covers `describe affected`,
`describe stacks`, and `list` consistently. For a disabled component: `!terraform.*` values remain raw
strings and `atmos.Component` returns empty sections. The gate keys strictly on `metadata.enabled` and
never on `vars.enabled` — a component disabled only at the Terraform-module level (`vars.enabled: false`) still has its functions evaluated.

## Example

A stack containing a component disabled with `metadata.enabled: false` whose config references an
unprovisioned component via `!terraform.state` now lets `atmos describe affected` complete instead of
failing with `terraform state not provisioned`.
