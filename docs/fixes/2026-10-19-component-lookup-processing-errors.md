# Fix: Report component processing errors instead of "component not found"

**Date:** 2026-10-19

## Summary

When a component is defined in the requested stack but fails to process, name-pattern and
name-template stack lookups now report that processing error. Previously every command that
resolves a single component (`describe component`, `terraform plan`, `!terraform.state`,
hooks) answered with the generic "Could not find the component" message.

## Context

The v1.232.0 → main regression test (`.context/regression-test/2026-10-19-v1.232.0-main`,
finding C5) added a component-level `retry:` block with an unknown key. Main rejects unknown
retry keys (#3295), but `atmos describe component y -s dev` printed
`invalid component: Could not find the component y in the stack dev` while the custom-command
and workflow paths named the offending key. `findComponentInStacks` ran
`processComponentConfig` for every stack and treated any error as "not defined here", so the
decode error was discarded before the caller built the not-found message. Directory-type
stacks were already fixed for the same problem on 2026-10-14; this closes the gap for the
default lookup path.

## Changes

- `internal/exec/utils.go`: `findComponentInStacks` checks the merged manifest to tell an
  expected miss (component not defined in that stack) from a processing failure. A failure
  in a stack whose file name, base name, or manifest `name` equals the requested stack is
  returned as the lookup error. A failure in any other stack is returned as a hint, and
  `processStacks` appends it to the not-found message as a `Note:` line, so a genuine
  "not found" stays a `ErrInvalidComponent` while still exposing the cause.
- `internal/exec/component_lookup_processing_error_test.go`: regression test covering the
  matched-stack error (expects `ErrInvalidRetryConfig`, the `components.terraform.app.retry`
  path, and the unknown key) and the unmatched-stack hint (expects `ErrInvalidComponent` plus
  the other manifest's name and the key).

## Validation

- The new test failed before the fix with the generic not-found message and passes after.
- `go test ./internal/exec -count=1` passes.
- Patch lint reports zero findings across the combined fixes.
- End to end with a fresh build against the regression-test fixture:
  `'components.terraform.y.retry' in the stack manifest 'dev': invalid retry configuration:
  unknown retry field "backoff" (valid fields: …)`.

## Follow-ups

Stacks whose name comes only from `name_template`/`name_pattern` and whose file name differs
from the computed name fall into the hint path rather than the direct error, because the stack
name cannot be computed when processing fails. The cause is still shown in the `Note:` line.
