# Fix: custom command steps honor execution controls

**Date:** 2026-07-06

## Problem

Custom command steps had several execution gaps:

- per-step `working_directory` was not applied consistently;
- `output: none` did not fully suppress shell step output;
- step env maps could lose original key case before reaching extended step
  handlers;
- a failed step stopped later `when: failure()` style conditions from seeing
  the failure state.

These gaps made custom command steps behave differently from workflow steps.

## Fix

Custom command execution now resolves each step's working directory, suppresses
shell output when requested, restores step env key case before passing env to
extended step handlers, and tracks step failure status for later `when`
conditions.

## Tests

```shell
go test ./cmd ./pkg/runner/step ./pkg/schema -run 'Test.*CustomCommand|Test.*WorkingDirectory|Test.*OutputMode|Test.*Condition|Test.*Task' -count=1
```
