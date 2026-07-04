---
name: atmos-hooks
description: "Atmos hooks: lifecycle events, hook kinds, command/store/git/security hooks, scoping and overrides, toolchain integration, --skip-hooks, and Atmos Pro/local output"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Hooks

Use this skill for lifecycle hooks that run before or after component operations.

Hooks can run scanners, policy checks, store writes, Git actions, custom commands, or other
toolchain-aware automation around Terraform, Helm, Kubernetes, and other component commands.

## Related Skills

| Need | Load |
|---|---|
| Store output hooks | [atmos-stores](../atmos-stores/SKILL.md) |
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

## Common Events

Use before/after events for component operations, for example:

- `before.terraform.init`, `after.terraform.init`
- `before.terraform.plan`, `after.terraform.plan`
- `before.terraform.apply`, `after.terraform.apply`
- `before.terraform.deploy`, `after.terraform.deploy`
- `before.terraform.test`, `after.terraform.test`

Check local docs when using Helm, Kubernetes, or newly added component families because event names
follow the component command surface.

## Hook Kinds

Common hook kinds include `command`, `store`, `git`, `infracost`, `trivy`, `checkov`, and `kics`.
Use the specific kind when Atmos has one; use `command` for project-specific scripts.

Hooks can use `dependencies.tools` so required scanners or CLIs are installed and placed on `PATH`
for the hook execution context.

When the hooked component declares the hook binary in `dependencies.tools`, do not add a separate
`atmos toolchain install` step. Atmos resolves, installs, and injects the tool before the hook fires.

## Operational Guidance

- Use hooks for repeatable lifecycle behavior, not one-off local scripts.
- Scope hooks as narrowly as possible: component hooks for component-specific behavior, shared
  mixins/defaults for organization-wide checks.
- Use `--skip-hooks` to bypass all hooks for a diagnostic run, or `--skip-hooks=name1,name2` to skip
  specific hooks when supported by the command.
- Treat hook output as part of CI evidence. When Atmos Pro is connected and the hook kind supports
  upload, prefer structured upload; otherwise rely on local/CI summaries.
- Keep destructive hooks opt-in and visible in stack config.
