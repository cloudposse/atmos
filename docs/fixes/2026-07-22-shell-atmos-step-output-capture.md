# Fix: `shell` and `atmos` step output capture

**Date:** 2026-07-22

## Summary

Named `shell` and `atmos` steps now expose their successful command output to
later custom-command and workflow steps through the documented `.steps.<name>`
context.

## Context

Legacy command execution streamed output directly to the terminal but did not
store a step result. A later output step such as `markdown` therefore failed to
resolve `.steps.<name>.value`, even though the producer had completed
successfully. The defect is tracked by
[cloudposse/atmos#2781](https://github.com/cloudposse/atmos/issues/2781).

## Changes

- Captured successful `shell` and `atmos` output without replacing their
  existing cross-platform execution paths.
- Stored trimmed stdout as the primary value, preserved masked stdout, stderr,
  and exit code as result metadata, and evaluated declared outputs separately.
- Preserved live output, retries, custom-command shell `output: none`,
  terminal-attached sessions, process cleanup, and failure propagation.
- Evaluated declared outputs once after a successful command, outside the retry
  envelope, so an output-template error cannot repeat a completed command.
- Added capture support for host, persistent workflow-container, and per-step
  container shell execution.

## Validation

- Added regressions for `shell` and `atmos` output consumed by `markdown` in
  custom commands and workflows.
- Added custom-command and workflow regressions proving output-template errors
  do not retry successful `shell` or `atmos` commands.
- Audited the implementation against the step-output contract in
  `container-actions-and-step-outputs.md` and the command-step behavior in
  `workflow-step-types.md`.
- Ran complete coverage-enabled tests for `./cmd`, `./internal/exec`,
  `./pkg/runner/step`, and `./pkg/workflow`; patch-scoped coverage reported no
  uncovered added behavior.
- Ran the focused regressions with the race detector and shuffled test order.
- Re-ran the original custom-command and workflow reproductions; both rendered
  the producer's value in the later `markdown` step.
- Ran `go build ./...` and patch-scoped lint against `upstream/main` with no
  findings.
- Built the Docusaurus website and validated this fix record.
- `atmos test` and `atmos test --coverage` reached and passed all touched
  packages. Their full-repository runs remained nonzero because of unrelated
  AWS/XDG environment assertions and a transient vendoring pull; the transient
  tests passed when isolated.

## Follow-ups

None.
