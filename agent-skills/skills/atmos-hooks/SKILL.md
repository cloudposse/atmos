---
name: atmos-hooks
description: "Atmos hooks: lifecycle events, hook kinds, command/store/git/security hooks, step/steps hooks, when: conditions, scoping and overrides, toolchain integration, --skip-hooks, and Atmos Pro/local output"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Hooks

Use this skill for lifecycle hooks that run before or after component operations,
or for generation hooks declared in a scaffold template.

Hooks can run scanners, policy checks, store writes, Git actions, custom commands, or other
toolchain-aware automation around Terraform, Helm, Kubernetes, and other component commands.

## Related Skills

| Need | Load |
|---|---|
| Store output hooks | [atmos-stores](../atmos-stores/SKILL.md) |
| Shared step fields and `kind: step` payloads | [atmos-steps](../atmos-steps/SKILL.md) |
| Git hooks and GitOps repositories | [atmos-git](../atmos-git/SKILL.md) |
| Tool installation for hook commands | [atmos-toolchain](../atmos-toolchain/SKILL.md) |
| CI summaries and Atmos Pro upload | [atmos-ci](../atmos-ci/SKILL.md) and [atmos-pro](../atmos-pro/SKILL.md) |

## Hook Shape

Hooks are configured in stack manifests at global, component-type, or component scope.

```yaml
hooks:
  store-vpc-outputs:
    events:
      - after.terraform.apply
    kind: store
    name: prod/ssm
    outputs:
      vpc_id: .vpc_id

components:
  terraform:
    vpc:
      hooks:
        scan-plan:
          events:
            - after.terraform.plan
          kind: trivy
```

Modern dotted event names such as `after.terraform.plan` are preferred. Legacy hyphenated event
names may appear in older stacks; modernize them when editing nearby config.

## Lifecycle Events

Use before/after events for component operations, for example:

- `before.terraform.init`, `after.terraform.init`
- `before.terraform.plan`, `after.terraform.plan`
- `before.terraform.apply`, `after.terraform.apply`
- `before.terraform.deploy`, `after.terraform.deploy`
- `before.terraform.test`, `after.terraform.test`

Kubernetes provides `before`/`after` events for `render`, `diff`/`plan`, `apply`/`deploy`,
`delete`, and `validate`. Native Helm provides `template`, `diff`, `apply`/`deploy`, and
`delete`; Helmfile provides `template`, `diff`, `apply`/`sync`/`deploy`, and `destroy`.
Use the canonical dotted events and remember that command aliases normalize to their execution
event (`deploy` to `apply`, Kubernetes `plan` to `diff`, and Helmfile `sync` to `apply`).

Scaffold templates use the separate `before.scaffold.generate` and
`after.scaffold.generate` events. They reuse the condition vocabulary but can run only
`kind: step` and `kind: steps`; do not configure stack-only kinds in `spec.hooks`.

Multi-component DAG runs (e.g. `--affected`, `--query`, or workflows that fan out across several
components) also fire aggregate events once for the whole run, in addition to the per-component
events fired for each individual component: `after.terraform.plan.aggregate`,
`after.terraform.apply.aggregate`, and `after.terraform.destroy.aggregate`. Use a per-component event
for component-specific behavior (scans, store writes) and an aggregate event for run-level summaries
or notifications that should fire only once.

## Conditional Execution with `when`

Hooks share the same `when:` condition engine as workflow steps: predicate keywords (`ci`, `local`,
`always`, `never`, `success`, `failure`) or a CEL expression built from runtime facts such as `stack`
and `component`. For example, restrict a hook to CI runs against the `prod` stack:

```yaml
hooks:
  prod-ci-scan:
    events:
      - after.terraform.plan
    kind: trivy
    when: stack == "prod" && ci
```

See [atmos-workflows](../atmos-workflows/SKILL.md#conditional-execution-with-when) for the full
`when:`/CEL syntax reference.

## Hook Kinds

Stack lifecycle hooks support `command`, `store`, `git`, `infracost`, `trivy`, `checkov`,
`kics`, and the step bridge. The legacy `ci.*` hook kinds still parse but are deprecated no-ops;
use the current CI provider bindings instead. Use a named kind when Atmos has one; use `command`
for a project-specific binary. The legacy `command:` discriminator and hyphenated events remain
compatibility input only; author new configuration with `kind:` and dotted events.

Hooks can use `dependencies.tools` so required scanners or CLIs are installed and placed on `PATH`
for the hook execution context.

When the hook declares the required binary in `dependencies.tools`, do not add a separate
`atmos toolchain install` step. Atmos resolves, installs, and injects the tool before the hook fires.

## Step-Backed Hook Kinds

Hooks can also delegate to the same step-type registry that workflows, custom commands, and cast
recordings use, instead of one of the named kinds above:

- `kind: step` runs **one** registered step type. Set the step type with the hook's `type:` field and
  configure it with `with:`, exactly like a workflow step.
- `kind: steps` runs an ordered list of registered step types, provided as a YAML list under `with:`.

Both run strictly in order -- there is no concurrent execution within a step-backed hook.

The hook envelope owns `events`, `when`, `env`, `retry`, and `on_failure`; `with:` is
decoded and validated as the step's own configuration. `kind: step` supplies the one
step type through the hook's `type:`; `kind: steps` supplies type-bearing objects in its
ordered `with:` list.

```yaml
hooks:
  check-prereqs:
    events:
      - before.terraform.plan
    kind: step
    type: require
    with:
      tools:
        - kubectl
        - helm

  bring-up-and-plan:
    events:
      - before.terraform.plan
    kind: steps
    with:
      - type: emulator
        command: up
      - type: atmos
        command: terraform plan vpc
```

Use `kind: step`/`kind: steps` when you need a registered step type (`container`, `emulator`,
`require`, `atmos`, `shell`, and other types workflows support) inside a hook; use the older named
kinds (`trivy`, `checkov`, `kics`, `infracost`) when Atmos already ships a purpose-built scanner
integration for the job.

## Operational Guidance

- Use hooks for repeatable lifecycle behavior, not one-off local scripts.
- Scope hooks as narrowly as possible: component hooks for component-specific behavior, shared
  mixins/defaults for organization-wide checks.
- Use `--skip-hooks` to bypass all hooks for a diagnostic run, or `--skip-hooks=name1,name2` to skip
  specific hooks by name. This flag is registered on the `terraform` command only today; there is no
  helmfile or packer equivalent yet.
- Treat hook output as part of CI evidence. When Atmos Pro is connected and the hook kind supports
  upload, prefer structured upload; otherwise rely on local/CI summaries.
- Keep destructive hooks opt-in and visible in stack config.
