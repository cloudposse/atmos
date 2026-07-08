---
name: atmos-workflows
description: "Workflow automation: native step types, multi-step workflows, parallel/matrix/wait/container/emulator steps, when: conditions (CEL), require/assert preconditions, output steps, retries, dependencies, and cross-component orchestration"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
references:
  - references/workflow-syntax.md
---

# Atmos Workflows

Use this skill for reusable orchestration in `workflows:` files: multi-step deployment flows,
parallel or matrix execution, cross-component operations, preconditions, retries, typed UI/output
steps, container/emulator steps, and workflow-level dependencies.

For full workflow syntax, read [references/workflow-syntax.md](references/workflow-syntax.md).

## Quick Shape

```yaml
workflows:
  deploy-network:
    description: Deploy network components
    stack: plat-ue2-dev
    steps:
      - type: atmos
        command: terraform deploy vpc
      - type: atmos
        command: terraform deploy dns
```

```shell
atmos workflow deploy-network
atmos workflow deploy-network --stack plat-ue2-prod
```

## Discovery

Workflow files live under `workflows.base_path` in `atmos.yaml`.

```yaml
workflows:
  base_path: stacks/workflows
```

When `--file` is omitted, Atmos scans workflow files and runs the workflow if exactly one match is
found. Use `--file` for ambiguous workflow names.

## Step Type Guidance

Use native step types when they express the intent directly:

| Need | Prefer |
|---|---|
| Run Atmos | `type: atmos` |
| Shell/process execution | `shell` or `exec` |
| Concurrent execution | `parallel`, `matrix` |
| Background services and waits | `background: true`, `wait`, `wait-all` |
| Preconditions | `require` / `assert` |
| Retry transient failures | `retry` |
| Containers and emulators | `container`, `emulator` |
| HTTP calls | `http` |
| User-facing output | `say`, `toast`, `markdown`, `table`, `pager`, `format`, `spin`, `stage` |
| Workflow recordings | `cast`, `simulate` via `atmos-cast` |

Shell is appropriate for short glue, terminal-native tools, or checked-in scripts. Large inline
shell blocks with loops, sleeps, formatting, CI metadata, or hand-rolled parallelism should usually
be replaced by native workflow steps.

## Conditions

`when` uses built-in predicates or CEL expressions:

```yaml
steps:
  - name: prod-only
    type: shell
    command: ./scripts/check-prod.sh
    when: !cel 'stack == "prod" && ci'
```

Built-in predicate keywords include `ci`, `local`, `always`, `never`, `success`, and `failure`.
Use `!cel` when a condition should be evaluated as CEL rather than treated as a predicate keyword.

`when: manual` is not an Atmos workflow predicate. For approvals, use a plan/apply split and CI
environment protection rules.

## Preconditions

`require` and `assert` verify required tools, files, dirs, environment variables, commands, or HTTP
resources before continuing. They do not install anything.

```yaml
steps:
  - type: require
    tools:
      - terraform
    files:
      - atmos.yaml
```

Route tool installation to `atmos-toolchain`.

## Parallel and Matrix

Use `parallel` for independent steps:

```yaml
steps:
  - type: parallel
    max_concurrency: 4
    fail:
      mode: wait_all
    steps:
      - type: atmos
        command: terraform plan vpc
      - type: atmos
        command: terraform plan dns
```

Use `matrix` when the workflow expands axes into repeated steps.

## Dependencies

Declare workflow tool dependencies in the workflow or step context:

```yaml
workflows:
  scan:
    dependencies:
      tools:
        checkov: "latest"
    steps:
      - type: shell
        command: checkov --directory .
```

Atmos toolchain installs and exposes declared tools for the workflow execution context.

## Auth

Use `identity` on a workflow or step when a command needs Atmos Auth credentials:

```yaml
steps:
  - type: shell
    identity: prod-readonly
    command: aws sts get-caller-identity
```

Route provider, identity, OIDC, assume role/root, and profile details to `atmos-auth` and
`atmos-profiles`.

## Routing

| Need | Skill |
|---|---|
| Complete workflow schema and examples | [references/workflow-syntax.md](references/workflow-syntax.md) |
| Custom CLI commands under `commands` | `atmos-custom-commands` |
| Cast/simulate workflow recordings | `atmos-cast` |
| Tool installation and PATH behavior | `atmos-toolchain` |
| Auth identities and providers | `atmos-auth` |
| Component dependencies and deployment order | `atmos-components`, `atmos-terraform` |
| CI approvals, matrices, outputs | `atmos-ci` |

## Guardrails

- Keep reusable orchestration in workflows, not ad hoc scripts.
- Prefer `atmos terraform deploy` for deployment steps so dependencies can be honored.
- Do not use sleeps for readiness if a `wait`, health check, or `require` step can express it.
- Avoid hidden state between steps; pass explicit outputs or files.
