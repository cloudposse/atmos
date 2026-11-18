# Fix: `docs generate` hooks field-test findings

**Date:** 2026-11-17

## Summary

Hooks on `atmos docs generate` (`before.docs.generate` and `after.docs.generate`) had six defects found in
a field test: the docs environment variables never reached hook subprocesses, hooks ran in random order,
hook failures did not name the hook, a list-shaped `hooks:` produced an opaque decoder error, an invalid
step hook only warned at run time, and a few error and documentation messages were unhelpful.

## Context

The hook engine builds an `ExecContext` for each hook. For repo-level operations, the new
`OperationEnv` field (carrying `ATMOS_DOCS_TYPE`, `ATMOS_DOCS_OUTPUT_PATH`, `ATMOS_DOCS_BASE_DIR`) was read
when building the subprocess environment but was never assigned when the context was created. Hook
execution also iterated a Go map, so order changed between runs. The remaining findings were missing
validation and messaging around operation hooks.

## Changes

- Assign `OperationEnv` when building the execution context, covering user hooks, policy-selected hooks,
  and the step engine (which builds its environment through the same function).
- Run, preflight and verify hooks in hook-name order. This also makes component hook order deterministic.
- Name the hook and event in failure errors (`hook "name" (kind command, event ...) failed`), keeping the
  wrapped error so the hook's exit code still propagates.
- Make the missing-binary hint operation-aware: it no longer mentions Terraform, and omits the
  `dependencies.tools` suggestion for repo-level operations that have no component dependencies.
- Reject hooks that are not maps with an error that names the config path
  (`docs.generate.<type>.hooks`) and shows an example.
- Validate operation hooks when they are constructed, using the same checks governing policy hooks get,
  so an unknown kind, unknown step type or missing required step field fails before generation.
- Add a hint to the unknown doc type error listing the configured `docs.generate` types.
- Log a warning when a docs hook declares events from another operation, since they can never fire.
- Document in the docs page and the PRD that hooks run in hook-name order and with the project root as the
  working directory, not `base-dir`.

## Validation

- Each fix was reproduced with a failing test first (environment variables, ordering, error text and hint,
  non-map hooks, construction-time validation, doc type hint), then the code was fixed. The foreign-event
  warning was added with its test in the same step.
- New tests run a real command-hook subprocess through the test binary and assert the three variables
  arrive for both a user hook and a policy hook.
- `go test -count=1` passed for `./pkg/hooks/...`, `./pkg/docs/...`, `./errors/...`, `./pkg/policy/...`,
  `./pkg/config/schema`, `./pkg/schema`, `./cmd/terraform`, `./pkg/component/...`, and `./cmd` with
  `-run Docs`.
- `atmos lint --changed` reported 0 issues. No golden snapshots changed.
- `go test` prints `[no tests to run]` for `pkg/hooks`; a test re-runs the test binary as a child process
  that prints the warning. All tests still run and pass.

## Follow-ups

None.
