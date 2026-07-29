# Fix: Control `env` step process exports

**Date:** 2026-07-27

## Summary

`type: env` values always become available to later step templates. They reach
later command subprocesses by default, including shell, Atmos, and control-step
children; `export: false` keeps a value template-only.

## Context

The runner previously used one environment map for both template resolution and
subprocess execution. This made the intended scope of an env-step assignment
unclear and did not propagate it consistently to ordinary workflow commands.

## Changes

- Split template and process environment state while preserving default export
  behavior for existing env steps.
- Applied exported values to workflow and control-step command environments,
  with step-level `env` retaining precedence.
- Applied the same behavior to ordered step hooks, including deferred payload
  rendering and retry/invocation isolation.
- Added unit and CLI coverage for exported and template-only values.

## Validation

- `go test ./internal/exec -run 'TestPrepareStepEnvironment|TestExecuteWorkflowControlStepUsesResolvedIdentityFallback' -count=1` — passed.
- `go test ./internal/exec -count=1` — passed.
- `go test -v ./tests -run 'TestCLICommands/atmos_workflow_env_step_propagates_to_(shell|atmos)$' -count=1` — passed.
- `go test ./tests -run TestTestCaseSchemaValidation -count=1` — passed.
- `cd website && npm run build` — passed (existing broken-anchor warnings remained).
- `bash .claude/skills/fix-log/scripts/validate-fix-doc.sh docs/fixes/2026-07-27-workflow-env-step-propagation.md` — passed.

## Follow-ups

None.
