# Fix: `kind: step` and `kind: steps` hooks honor per-step `when`, `inputs`, and `artifacts`

**Date:** 2026-10-22

## Summary

Lifecycle hooks that delegate to the shared step library executed every step unconditionally. A
step-level `when:` was never evaluated, and `inputs`/`artifacts` freshness declarations (implicit or
explicit `when: checksum.changed`) were silently ignored, so a step such as `npm ci` re-ran on every
hook invocation even though the same step skips correctly in a custom command or workflow. Hook steps
now evaluate their effective condition with the same lifecycle and freshness facts, skip when it is
false, and record freshness state after success.

## Context

Field-testing the CDK synthesis tutorial showed that an install step declared with
`inputs.sources: [package-lock.json]` and `artifacts.paths: [node_modules/.bin/cdk]` inside a
`kind: steps` hook reinstalled on every `diff`. Freshness was wired only in `cmd/cmd_utils.go`
(custom commands) and `internal/exec/workflow_utils.go` (workflows); `pkg/hooks/step_engine.go`
passed each step straight to the executor, which does not evaluate `when:` itself. The step docs
present the library as shared across workflows, custom commands, and hooks, so the gap was a silent
no-op that validated and did nothing.

## Changes

- `pkg/hooks/step_freshness.go` (new): `hookStepPolicy` builds a condition context from the hook
  (`status`, `component`, `stack`, `hook`, `event`, step name, hook `env`), computes freshness facts
  through `pkg/runner/freshness` keyed by component, stack, and hook name, evaluates the effective
  `when:`, and records success. A step with neither a `when:` nor freshness declarations always runs.
  Only an explicit step `when:` consults the operation outcome, so an implicit `checksum.changed`
  never re-applies the envelope's default `when: success` inside a `when: failure` hook.
- `pkg/hooks/step_engine.go`: both engines consult the policy before each execute and record
  success afterwards; skipped steps are logged at debug level.
- `pkg/hooks/step_engine_freshness_test.go` (new): per-step `when` predicates (`never`, `failure`,
  CEL on component and stack), freshness skip/rerun across runs for `kind: steps`, and the same for
  `kind: step`, using an uncached globber so in-process artifact removal is observed.
- `website/docs/stacks/hooks.mdx` and `website/docs/steps/index.mdx`: document that per-step
  `when`, `inputs`, and `artifacts` behave in hooks as elsewhere, with an install-then-synth example.
- The CDK tutorial and the `cdk-cloudformation` / `cdk-terraform` examples use the native
  `inputs`/`artifacts` form for their install step instead of a shell guard.

## Validation

- `go test ./pkg/hooks` passes, including the new tests and the pre-existing
  `TestStepEngineSeedsOutcomeEnv`, which caught the first draft suppressing after-failure hooks.
- `atmos lint --changed` reports no issues (run with the examples' `node_modules` removed; the Go
  template files CDK ships inside `node_modules` otherwise fail typecheck locally).
- Live against the Floci emulator in `examples/cdk-cloudformation`: the first `diff` installs
  (`added 12 packages`), the second skips the install and only synthesizes, appending to
  `package-lock.json` reruns the install, and `.atmos/cache/freshness/` records the state.
- `atmos test` in both examples passes end to end with the `type: test` smoke checks.

## Follow-ups

- The production glob engine is LRU-cached per process, so a step that deletes its own artifact and
  re-evaluates within the same process can see a stale match. Real CLI runs start a fresh process per
  invocation, so this is not observable today; it is pre-existing in `pkg/runner/freshness` and not
  introduced here.
