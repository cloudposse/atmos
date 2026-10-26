# Fix: Freshness-referencing `when:` conditions no longer silently skip the whole command/workflow

**Date:** 2026-08-05

## Summary

`executeCustomCommand` (custom commands) and `ExecuteWorkflow` (workflows) each run a cheap
pre-check before executing any steps: evaluate every step's `when:` against a lightweight
`Context` to decide whether anything would run at all. That pre-check never populated freshness
facts (`checksum.changed`/`timestamp.changed`/`precondition.success`/`sources`/`artifacts`), so a
step with an *explicit* freshness-referencing `when:` (e.g. `when: timestamp.changed`, or
`when: "sources.exists(...)"`) always evaluated false there, regardless of actual file state —
silently skipping the entire command/workflow before the real, correctly-wired per-step loop
(which does compute and pass real freshness facts) ever ran. Both pre-checks now recognize when a
step's effective `when:` references a freshness fact and defer the decision to the real loop
instead of evaluating against empty facts.

## Context

Discovered while field-testing the task-runner/dependency-graph feature
(`osterman/task-runner-first-class-support`): `atmos freshbuild-ts`/`atmos smart-build` (custom
commands with `inputs:`/`artifacts:` and an explicit `when:` override) exited 0 with no error, but
their steps never ran — debug logs showed `Skipping custom command, no steps matched \`when\`
conditions`. The implicit default (`checksum.changed`, synthesized only when `inputs:`/
`artifacts:` are set with no explicit `when:`) was unaffected, since a zero `Condition`'s
`EvaluateWithImplicitSuccessE` only checks `status == success` (always true) — it never reaches
this bug. Only an *explicit* freshness-referencing `when:` triggers it.

The identical pattern existed in `internal/exec/workflow_utils.go`'s `needsAuth` pre-check, with a
narrower blast radius (only gates whether an auth manager gets set up, not whether the whole
workflow runs) — fixed alongside for consistency, same root cause.

Note: while investigating, an *apparent* second symptom (`atmos workflow release -f
task-runner.yaml` throwing a hard "undeclared reference to 'timestamp'" CEL compile error) turned
out to be an unrelated bug — see the separate
`2026-08-05-workflow-command-dependency-wrong-atmos-binary.md` fix doc. It was not caused by this
pre-check at all.

## Changes

- `pkg/runner/freshness/checker.go`: added `MentionsAnyFreshnessFact(when schema.Condition) bool`,
  a small exported helper (reusing the existing `Condition.MentionsCELIdentifier` and the package's
  own identifier list) so both call sites share one implementation instead of duplicating it.
- `cmd/cmd_utils.go`'s `hasRunnableStep` pre-check: for each step, compute
  `freshness.EffectiveWhen(step.When, declared)` and check
  `freshness.MentionsAnyFreshnessFact(effective)` first; if true, treat the step as
  possibly-runnable without evaluating it there, deferring to the real per-step loop
  (`cmd_utils.go:918-944`) that already passes real freshness facts. Falls through to the existing
  evaluation for plain conditions (`ci`/`status`/`env`/`os`/`arch`/`platform`).
- `internal/exec/workflow_utils.go`'s `needsAuth` pre-check: identical treatment.

## Validation

- New tests in `cmd/custom_command_inputs_test.go`:
  `TestCustomCommandIntegration_ExplicitWhenTimestampChangedRunsOnFreshState` and
  `TestCustomCommandIntegration_ExplicitWhenSourcesArtifactsCompareRunsOnFreshState`. Both
  confirmed failing before the fix (`unable to find file ".../run.txt"` — the step never ran) and
  passing after. Note: `timestamp.changed` and the structured `sources`/`artifacts` comparison are
  both stateless, pure current-mtime comparisons (no "first run" concept, unlike
  `checksum.changed`'s persisted-state model) — both tests set explicit `os.Chtimes` so the source
  is unambiguously newer than the artifact, rather than relying on "no prior state."
- Existing tests unaffected: `TestCustomCommandIntegration_InputsSkipsWhenSourcesUnchanged`,
  `TestCustomCommandIntegration_Precondition{SkipsWhenToolAlreadyOnPath,RunsWhenToolMissing}`,
  `TestExecuteWorkflow_InputsSkipsWhenSourcesUnchanged`,
  `TestExecuteWorkflow_PreconditionSkipsWhenToolAlreadyOnPath`,
  `TestExecuteWorkflow_DependenciesWorkflows{SameFile,CrossFile}` — all still pass.
- End-to-end: rebuilt `atmos`, ran `atmos freshbuild-ts` and `atmos smart-build` against
  `examples/task-runner-dependencies/` from a clean state (no prior artifact) — both now execute
  their steps instead of silently exiting 0 with nothing run.

## Follow-ups

None.
