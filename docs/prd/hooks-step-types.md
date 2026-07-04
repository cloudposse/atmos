# PRD: Run custom step types as component lifecycle hooks

## Status

Implemented (single PR). Depends on the `http` step type PR for the Slack/webhook example to work end-to-end; the bridge itself ships independently of `http`.

## Problem

Atmos has two execution subsystems that don't share capabilities:

- **Hooks** (`pkg/hooks/`) fire on Terraform lifecycle events (before/after `init`/`plan`/`apply`/`deploy`). A hook's `kind:` selects from a small fixed registry — `store`, `command`, `infracost`, `checkov`, `kics`, `trivy`, `git`. Anything outside that list (notify Slack, run a container, render a formatted summary) means falling back to `kind: command` and hand-rolling a shell invocation.
- **Step registry** (`pkg/runner/step/`) already offers ~25 rich, self-contained step types (`shell`, `atmos`, `container`, `log`, `format`, `table`, `markdown`, `toast`, `http` once it lands, …) powering workflows and custom commands.

Every new "thing a hook should be able to do" has historically meant adding a new hook kind. That doesn't scale and duplicates work the step registry already does.

## Goal

Let a hook delegate to **any** registered step type, so the entire step library becomes available on Terraform lifecycle events without growing the hook-kind list. Reuse the step registry rather than fork it (extend, don't fork).

## Design

A new hook kind, `step`, bridges to the step registry.

```yaml
hooks:
  notify-slack:
    kind: step
    type: http                 # any registered step type
    events:
      - after-terraform-apply
    on_failure: warn           # envelope policy: warn | fail | ignore
    retry:                     # envelope policy: same RetryConfig schema as a workflow step
      max_attempts: 3
    with:                      # the step's own parameters (templated + YAML-function rendered)
      url: https://hooks.slack.com/services/XXX
      method: POST
      body: '{"text": "Deployed {{ .Component }} to {{ .Stack }}"}'
```

### Envelope vs. `with:`

- The **envelope** (`kind`, `type`, `events`, `on_failure`, `retry`, `env`) is what the *hook runner* interprets.
- **`with:`** is what the *step handler* interprets — it decodes into a `schema.WorkflowStep`.

`on_failure` and `retry` live at the envelope because they are wrapper-level policy: the bridge applies them *around* the step handler (`retry.Do` + `ApplyOnFailure`), exactly as `pkg/runner/runner.go` does when running workflow steps. The step itself never sees them.

### Dispatch

`kind: step` is a normal registered kind (`pkg/hooks/step_engine.go`, `init()` → `RegisterKind`). `Hooks.RunAll` dispatches to its engine unchanged. The engine:

1. Validates `type` names a registered step type.
2. Builds a `schema.WorkflowStep` by round-tripping the rendered `with:` map through YAML into the step struct (reuses the step's existing YAML tags and nested-struct decoding), then forces `Type`/`Retry` from the envelope.
3. Seeds a `step.Variables` env with the standard `ATMOS_*` variables (`BuildAtmosEnv`, shared with the command kind) plus the hook's `env:`.
4. Runs the step via the same `step.StepExecutor` workflows/custom-commands use, wrapping in `retry.Do` when `retry:` is set.
5. Maps failure through `ApplyOnFailure` (warn/fail/ignore).

`with:` rendering is free: `resolveHookForExecution` already recurses through nested maps/slices applying templates and YAML functions, so `{{ .Component }}` / `!store …` inside `with:` are resolved before the step runs.

### Operation outcome (success/failure context)

A key use case is announcing what happened ("the VPC component in the foobar stack failed"). Three pieces make it work:

1. **User hooks fire on the failure path too.** Cobra skips `PostRunE` on error, so today user hooks (`hooks.RunAll`) only run on success — only CI hooks ran on failure. The RunE error defer now also calls `runUserHooks` with a failure outcome (single-component mode; multi-component already suppresses global user hooks). Errors there are advisory and never mask the original command error.

2. **`RunAll` carries an `Outcome`** (`Status` success/failure, `Err`, `ExitCode`) via a functional option `WithOutcome` (variadic — existing callers unchanged). It lands on `ExecContext.Outcome`.

3. **A `when` selector** (`success` | `failure` | `always`, default `success`) filters each hook against the outcome. Default success-only preserves back-compat — a `store` hook never runs after a failed apply. `when` is generic across all hook kinds.

The outcome is exposed two ways (both, per design):
- **Template context** in `with:` — `{{ .status }}`, `{{ .exit_code }}`, `{{ .error }}` injected alongside the existing `{{ .atmos_component }}` / `{{ .stack }}` (component/stack were already present).
- **Env vars** — `ATMOS_HOOK_STATUS`, `ATMOS_HOOK_EXIT_CODE`, `ATMOS_HOOK_ERROR` (plus the existing `ATMOS_COMPONENT` / `ATMOS_STACK`), via `BuildAtmosEnv`, so they reach every hook kind.

`on_failure` (the hook's own failure handling) and `when` (operation-outcome filter) are orthogonal and documented as such.

### No guardrails

All step types are allowed, including interactive/TTY ones. Steps are responsible for their own headless/CI behavior; Atmos does not second-guess them. (A future change could add an opt-in policy, but v1 ships none.)

### Validation

- **Preflight** (before Terraform runs): `verifyStepHookType` confirms `type` is a registered step type so a typo fails fast. It does not render or validate `with:` (which may still contain unrendered templates pre-auth).
- **Run time**: the step handler's own `Validate()` is the real gate (the JSON schema keeps `with:` permissive — `additionalProperties: true`).

### JSON schema

The `hooks` definition (in `stacks/stack-config`, `atmos/manifest`, `config/global` 1.0.json) gains a structured per-hook envelope: a `kind` enum (incl. `step` and the deprecated `ci.*` kinds for back-compat), `events`, `on_failure` enum, `type`, `with`, `retry`, and the existing kind fields. Per-hook `additionalProperties` stays `true` (non-breaking; tightening to `false` is a tracked follow-up).

## Non-goals

- Adding the `http` step type (separate PR).
- Step output → hook artifact/Pro upload plumbing beyond a best-effort status `Summary`.
- Cancellation via the parent Terraform context (the bridge uses `context.Background()`, matching the existing command engine).

## Files

- `pkg/hooks/step_engine.go` — bridge engine, `kind: step` registration, preflight type check.
- `pkg/hooks/hook.go` — `Type`, `With`, `Retry` fields.
- `pkg/hooks/hooks.go` — preflight routes step kind to the type check.
- `pkg/hooks/command_engine.go` — `BuildAtmosEnv` exported.
- `pkg/datafetcher/schema/**/1.0.json` — structured hook envelope.
- Tests: `pkg/hooks/step_engine_test.go`.
- Docs: hooks reference page, changelog blog post, roadmap milestone.
