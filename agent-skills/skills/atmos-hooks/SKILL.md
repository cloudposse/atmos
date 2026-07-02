---
name: atmos-hooks
description: "Atmos lifecycle hooks for Terraform operations, store publishing, scanner hooks, git hooks, command hooks, and shared step hooks"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Hooks

Atmos hooks run automated actions around component lifecycle events. Use this
skill when configuring `hooks:` in stack manifests, debugging hook execution, or
deciding whether automation belongs in a hook, workflow, or custom command.

For `kind: step`, also load `atmos-steps`. Hooks share the same step library as
workflows and custom commands, but hooks add a lifecycle envelope around the
step.

## When To Use Hooks

Use hooks for automation that is lifecycle-adjacent:

- Publish Terraform outputs to stores after apply.
- Run cost, security, or policy scanners around plan/apply.
- Commit or push generated artifacts after successful operations.
- Send notifications after success or failure.
- Run cleanup or reporting tied to Terraform operation outcomes.

Prefer workflows or custom commands for user-invoked orchestration, multi-step
runbooks, and commands that should be discoverable as first-class CLI actions.
Do not hide broad deployment flows inside hooks.

## Hook Shape

Hooks are stack configuration. They can be defined globally, at the component
type level, or on a specific component, and Atmos deep-merges them through stack
inheritance.

```yaml
components:
  terraform:
    vpc:
      hooks:
        publish-outputs:
          kind: store
          events: [after.terraform.apply]
          name: ssm
          outputs:
            /network/vpc/id: .vpc_id
```

Use `website/docs/stacks/hooks.mdx` as the canonical user documentation and
inspect `pkg/hooks/` when behavior is unclear.

## Hook Kinds

Common hook kinds:

- `store`: Write selected Terraform outputs to a configured store.
- `command`: Run a custom command or external command.
- `git`: Commit or push generated artifacts.
- `step`: Run any registered workflow/custom-command step type.
- `infracost`, `checkov`, `trivy`, `kics`: Built-in scanner integrations.

The legacy `command:` field can alias hook kind in older examples. Prefer
explicit `kind:` in new configuration.

## Step Hooks

`kind: step` bridges hooks to the shared step DSL:

```yaml
hooks:
  notify-slack:
    kind: step
    type: http
    events: [after.terraform.apply]
    on_failure: warn
    retry:
      max_attempts: 3
    with:
      url: https://hooks.example.com/services/XXX
      method: POST
      body: '{"text": "Deployed {{ .atmos_component }} to {{ .stack }}"}'
```

Keep the separation clear:

- The hook envelope (`kind`, `type`, `events`, `on_failure`, `retry`, `when`,
  `env`) is interpreted by the hook runner.
- `with:` is interpreted by the selected step type and should contain the same
  fields you would place on that step in a workflow.

Atmos renders `with:` with the standard hook template context and YAML
functions, then the step validates its own parameters.

## Events And Conditions

Hooks run for lifecycle events such as Terraform plan/apply/deploy stages. Use
the docs for the exact supported event names before adding a new hook.

Outcome conditions:

- Default behavior is success-oriented for after hooks.
- `when: success` runs only after a successful operation.
- `when: failure` runs only after a failed operation.
- `when: always` runs for both outcomes.
- Conditions such as `ci` can be combined with outcome behavior.

`when` describes the Terraform operation outcome. `on_failure` describes what
Atmos should do if the hook itself fails.

## Failure Policy

Every hook can declare `on_failure`:

- `warn`: Log a warning and continue.
- `fail`: Propagate the hook error and abort.
- `ignore`: Suppress hook failure.

Scanner hooks often default to warning behavior. Use `fail` only when a hook
finding or error should block the operation.

## Environment And Tools

Hooks receive standard `ATMOS_*` variables such as `ATMOS_STACK`,
`ATMOS_COMPONENT`, and component path context. Add hook-specific environment
with `env` maps rather than inline shell exports.

Hooks can declare tool dependencies. Atmos installs and verifies those tools
through the toolchain before the hook fires. Do not hard-code project-local bin
paths in shell when the toolchain or inherited PATH should provide the binary.

Atmos also creates per-hook output paths such as `ATMOS_OUTPUT_DIR` and
`ATMOS_OUTPUT_FILE` for hook artifacts. Use those instead of manually creating
shared temp directories.

## Native Fields Over Shell

When implementing hook behavior:

- Use `kind: step` plus `atmos-steps` for native step types.
- Use `env` maps instead of inline `PATH=... command`.
- Use `output: none` on step hooks instead of shell redirection.
- Use `working_directory` or step-specific path fields instead of `cd`.
- Use Terraform/OpenTofu `rc` configuration for CLI config instead of writing
  temporary rc files in shell.
- Use store hooks for output publishing instead of custom scripts that parse
  Terraform output.

Shell command hooks are appropriate when integrating with a tool that has no
native hook or step support, but they should not be the default design.

## Skip And Preflight

Users can skip hooks with `--skip-hooks` or `ATMOS_SKIP_HOOKS`. The skip setting
propagates through nested Atmos operations.

Atmos preflights hooks before long Terraform operations when it can. A typo in a
`kind: step` `type` or missing tool should fail early instead of after a long
plan/apply.

## Related Skills

- `atmos-steps`: Step types and shared step fields.
- `atmos-stores`: Store backends and output publishing hooks.
- `atmos-toolchain`: Declaring hook tool dependencies.
- `atmos-terraform`: Terraform lifecycle and `--skip-hooks` behavior.
