# Workflow shell/atmos/exec steps do not interpolate `{{ .steps.* }}` / `{{ .env.* }}` / `{{ .flags.* }}`

**Date:** 2026-07-07 **Severity:** High — silently emits literal template text
instead of the captured value, so a workflow that passes a value from one step
to a `shell`/`atmos`/`exec` step runs with the wrong input. **Reproducer:**
`internal/exec/workflow_command_templating_test.go`

______________________________________________________________________

## Symptom

A workflow step that feeds an earlier step's output into a `shell`, `atmos`, or
`exec` command (or into that step's `env:`) received the raw template string:

```yaml
workflows:
  deploy:
    steps:
      - name: component
        type: format
        content: "vpc"
      - type: atmos
        command: terraform apply {{ .steps.component.value }} -auto-approve
```

`terraform apply {{ .steps.component.value }} -auto-approve` was passed through
verbatim (the literal `{{ .steps.component.value }}`) instead of
`terraform apply vpc -auto-approve`.

## Cause

Workflow step types handled inline in `ExecuteWorkflow` (`shell`, `atmos`,
`exec`) executed their raw `command`, and `prepareStepEnvironment` merged `env:`
values raw — neither was run through the step-variable template engine. Only
handler-routed step types (`toast`, `markdown`, `container`, the interactive
prompts, …) resolved `{{ .steps.* }}`, and custom command steps resolved both
command and `env:` via `stepVars`. Workflows were the outlier, so the documented
"use step outputs in subsequent steps" contract (see
`docs/prd/workflow-step-types.md`) did not hold for command steps. It went
unnoticed because the shipped examples only surface step values through
handler-routed display steps, never a raw `shell`/`atmos` command.

## Fix

`ExecuteWorkflow` now resolves inline step commands and `env:` values through
the same template engine custom command steps use (the full Atmos renderer with
Sprig/Gomplate functions and multi-pass rendering), so `{{ .steps.* }}`,
`{{ .env.* }}`, `{{ .flags.* }}`, and template functions behave identically in
workflows and custom commands.

- `Variables.ResolveWith` resolves a string with a per-call environment overlay,
  so a step's environment is visible as `{{ .env.* }}` without mutating the
  shared executor env (no cross-step leakage).
- The workflow step executor is configured with the same renderer, pass count,
  and flag protection as the custom command executor (`cmd/cmd_utils.go`).

Commands and `env:` values that contain no template markers are returned
unchanged.
