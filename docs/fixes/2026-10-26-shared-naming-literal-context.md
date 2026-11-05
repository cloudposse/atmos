# Fix: Preserve literal context in the shared naming resolver

**Date:** 2026-10-26

## Summary

The shared naming resolver restores literal variables for stack and workspace
context while retaining protected values in template inputs and source maps.

## Context

Naming resolution moved into `pkg/naming` so runtime commands and doctor migration
verification share one implementation. Stack literal restoration and unconditional
workspace context initialization needed to move with it. Without that integration,
legacy patterns used internal literal envelopes, logical stack selection missed
components, and template or filename naming could leave workspace context empty.

## Changes

- Decode a copy of variables when deriving legacy stack-pattern context.
- Populate context for explicit, template, pattern, and filename naming strategies,
  clearing stale values while preserving the original component variables.
- Keep exec wrappers delegated to the shared resolver and retain computed-identity
  validation and naming precedence.
- Preserve doctor migration assessment and its coverage reasons alongside YAML
  scope and import incompleteness; extract the reason lookup to keep scan coverage
  logic simple.

## Validation

Existing runtime naming regressions and new shared-resolver tests failed before
integration. After the fix, focused exec and naming tests passed, including literal
stack selection, workspace context, source isolation, processing toggles, and
null-filter preservation. Full `go test ./pkg/doctor ./pkg/naming -count=1` passed,
covering migration equivalence, computed identities, YAML scope diagnostics, and
incomplete coverage. The fix-record validator and whitespace check passed.

## Follow-ups

None.
