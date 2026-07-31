---
name: atmos-lint
description: "Atmos Terraform linting with TFLint: standalone `atmos terraform lint`, component-aware config discovery and toolchain versions, TFLint rule configuration, and lifecycle hooks/CI findings. Use when configuring, running, debugging, or documenting Terraform/OpenTofu linting in an Atmos project."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Linting

Use this skill to design or operate Terraform/OpenTofu linting in Atmos. Prefer
the native `atmos terraform lint` command for deliberate lint runs and the
`tflint` hook kind for linting as part of a Terraform lifecycle.

## Choose an execution mode

| Need | Use |
|---|---|
| Check one component or every component without plan/apply | `atmos terraform lint` |
| Check only changed component sources | `atmos terraform lint --affected` |
| Enforce lint before or after a Terraform action | `kind: tflint` hook |
| Run a provider-plugin initialization command or a bespoke script | `kind: command` hook or workflow step |

```shell
# All Terraform component directories, once each (default)
atmos terraform lint
atmos terraform lint --all

# A component; Atmos selects a stack context deterministically when needed
atmos terraform lint vpc
atmos terraform lint vpc --stack test

# Changed Terraform components only
atmos terraform lint --affected
```

Atmos resolves the selected component instance before linting. Declare `tflint`
under that component's `dependencies.tools` to pin the binary version; Atmos
installs it and supplies its PATH for that lint execution. A component used by
multiple stacks is linted once, with a deterministic stack context used only to
resolve its settings and toolchain.

```yaml
components:
  terraform:
    vpc:
      dependencies:
        tools:
          tflint: "0.59.1"
```

Do not use `atmos toolchain install` as a prerequisite for normal component
linting when `dependencies.tools` declares TFLint. Use it only to warm a cache
or troubleshoot an interactive shell. For precedence of tool declarations,
load [atmos-toolchain](../atmos-toolchain/SKILL.md).

## TFLint config discovery

When no hook or workflow explicitly passes `--config`, Atmos finds
`.tflint.hcl` in this order. The most-specific existing path wins:

1. Component directory
2. Terraform components base path (`components.terraform.base_path`)
3. Git repository root
4. `components.terraform.lint.config` (an absolute path or a path relative to the Atmos base path)

Use the explicit `lint.config` setting for a nonstandard shared config path:

```yaml
components:
  terraform:
    lint:
      config: config/tflint/company.hcl
```

The `tflint` hook and TFLint workflow step use the same discovery. An explicit
`--config` in their args is intentional and overrides discovery.

## Hooks and CI

Use a component hook when lint must be enforced for normal Terraform commands.
Choose the earliest event that gives the desired feedback; static TFLint checks
usually belong before init or plan.

```yaml
components:
  terraform:
    vpc:
      dependencies:
        tools:
          tflint: "0.59.1"
      hooks:
        lint:
          events:
            - before.terraform.plan
          kind: tflint
          on_failure: fail
```

The built-in hook runs TFLint with `--chdir=$ATMOS_COMPONENT_PATH` and
`--format=sarif`. Its default `on_failure: warn` reports findings without
blocking the Terraform command; set `on_failure: fail` to make lint gating.
SARIF is captured from stdout and can render terminal summaries, CI annotations,
and code-scanning results.

Use a `kind: command` hook for provider plugin initialization when required:

```yaml
hooks:
  tflint-init:
    events:
      - before.terraform.plan
    kind: command
    command: tflint --chdir=$ATMOS_COMPONENT_PATH --init
```

Keep initialization and lint separate so its network/plugin behavior is clear.
Load [atmos-hooks](../atmos-hooks/SKILL.md) for hook inheritance, conditions,
and failure handling; load [atmos-ci](../atmos-ci/SKILL.md) for CI integration.

## Rules and TFLint capabilities

Read [references/tflint.md](references/tflint.md) before changing rules, adding
provider plugins, or selecting TFLint flags. Preserve an existing project's
config style and run the exact component or `--affected` selection that verifies
the intended scope.
