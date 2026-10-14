# Fix: Quieter Starlark script output and a recursion circuit breaker

**Date:** 2026-10-04

## Summary

A field test of Atmos Automation Language found three output problems. Standalone
scripts printed the Atmos resource-usage summary after any `exec.run`. Script steps
printed `[step]` and `✓ step completed` labels that shell steps omit. Unbounded
recursion produced a 100,008-line traceback. Standalone scripts now show the summary
only when `settings.metrics.enabled: true` is set explicitly. Script steps default to
raw output with no labels, like command steps. A recursion depth limit stops runaway
recursion with a short, actionable error.

The same pass added an Automation Functions reference (one page per function) and a
language reference, rewrote the Custom CLI Apps introduction, and refreshed the
bundled `atmos-starlark`, `atmos-custom-commands`, and `atmos-steps` agent skills.

## Context

- `exec.run` accumulates subprocess metrics, so `DisplayFinalSummary` printed
  `▶ Total for this invocation …` at the end of every standalone script, even on
  success and outside a repository. A standalone script is the user's own CLI tool,
  so Atmos telemetry there must be opt-in.
- `ScriptHandler` chose its own output mode and defaulted to log mode with labels,
  while shell and Atmos command steps use `NewCommandOutputWriter`, which defaults to
  raw mode with labels off.
- starlark-go only stops recursion at 100,000 frames and reports the whole stack.
  It has no per-call depth hook.
- The docs showed `ℹ` for `ui.info`, which has rendered `▶` since June 2026, and used
  `dependencies.tools("jq", …)`, which fails with an ambiguous short-name error.

## Changes

- `cmd/standalone_script.go`: `suppressMetricsSummaryByDefault` sets
  `settings.metrics.enabled` to false for standalone scripts when unset. Explicit
  values are preserved. The field comment in `pkg/schema/metrics.go`, the generated
  config schema, and `settings/metrics.mdx` document the exception.
- `pkg/runner/step/script.go`: script steps build their writer with
  `NewCommandOutputWriter`, so `output:`, workflow `output:`, `show.labels: true`, and
  the pager setting still apply. Starlark CLI snapshots were regenerated; the only
  changes are removed label lines.
- `pkg/script/starlark/recursion.go`: every interpreter thread samples its call depth
  every 4096 steps and cancels past 10,000 frames. `scriptError` and parallel
  `taskError` classify the breaker and starlark-go's own overflow as
  `ErrStarlarkRecursionLimit` with a hint. `backtrace.go` collapses long runs of
  identical frames, including short recursion cycles, and leaves other tracebacks
  byte-for-byte unchanged.
- Docs: new `website/docs/functions/automation/` (one page per function, including
  every `atmos.<command>` wrapper) and `website/docs/automation/reference/` (lexical
  elements, data types, operators, statements, built-in functions, methods,
  execution model, differences from Python), wired into `sidebars.js`. The Custom
  CLI Apps page now leads with the Atmos capabilities scripts can use. Stale `ℹ`
  glyphs and the ambiguous `jq` tool name were corrected; a "Step labels" section
  was added to the script step reference.
- Agent skills: `atmos-starlark` was rewritten against the code, with reference files
  for standalone scripts, script steps, and the API; stale "spike" wording and links
  to `/workflows/steps/type` were fixed.

## Validation

- `go build ./...` passed.
- `go test ./cmd/ -run Standalone`, `./pkg/script/...`, `./pkg/workflow/...`,
  `./pkg/hooks/...`, `./pkg/ai/skills/...`, and
  `go test ./tests -run 'TestCLICommands/starlark'` passed.
- `go test ./pkg/runner/step/` passed except six cast session-mode tests that time out
  waiting for cast output. The same tests fail on a clean checkout of `HEAD` in this
  environment, so they are unrelated to this change.
- Manual runs with a rebuilt binary: the metrics line is absent by default and present
  with `enabled: true`; `recurse.star` exits 1 with a 15-line error naming
  `recursion depth exceeded (10000 frames)`; `dependencies.tools("jqlang/jq", "1.7.1")`
  installs jq and `jq --version` prints `jq-1.7.1`.
- `cd website && npm run build` passed with no broken links or anchors.

## Follow-ups

None.
