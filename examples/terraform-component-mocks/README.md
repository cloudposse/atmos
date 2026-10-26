---
title: Terraform Component Mocks
tags: [Terraform, Components]
related_docs:
  - label: "Terraform plan"
    url: /cli/commands/terraform/plan
  - label: "Component mock configuration"
    url: /stacks/components/mocks
---

# Terraform component mocks

This example lets `app` consume `vpc`'s output either from real Terraform state or from a [component-owned mock](https://atmos.tools/stacks/components/mocks).

Mocks are fallbacks by default. With `--use-mocks`, a `!terraform.state` lookup returns the real value when it exists and uses the component's `mocks` value only when the referenced component is not provisioned or the output is missing.

## Mock fallback (no state yet)

Before `vpc` has any state, `--use-mocks` fills in the missing value:

```shell
atmos describe component app -s dev --use-mocks
atmos terraform plan app -s dev --use-mocks
```

Both commands pass `vpc-local` from `vpc.mocks.vpc_id` into `app`.

## Real state wins once it exists

Create the producer's local state:

```shell
atmos terraform apply vpc -s dev
```

Now the same commands use the real `vpc-real` Terraform output, because real state takes precedence in fallback mode:

```shell
atmos describe component app -s dev --use-mocks
atmos terraform plan app -s dev --use-mocks
```

Without `--use-mocks`, lookups always use the normal state path and pass `vpc-real` into `app`.

## Always use mocks

To resolve from mocks only, even when state exists, pass `--use-mocks=always`:

```shell
atmos describe component app -s dev --use-mocks=always
atmos terraform plan app -s dev --use-mocks=always
```

Both commands pass `vpc-local` again. In `always` mode Atmos never initializes Terraform, authenticates, or reads a backend for those lookups.

To make `always` the default for the project, set it in `atmos.yaml`:

```yaml
components:
  terraform:
    mocks:
      mode: always
```

Projects pinned to a [config edition](https://atmos.tools/cli/configuration/edition) before 2026-10-01 get `always` by default, which is how bare `--use-mocks` behaved before mocks became fallbacks.

`--use-mocks` is accepted only by `atmos terraform plan` and `atmos describe component`; every other `atmos terraform` subcommand (for example `apply`, `deploy`, and `destroy`) rejects it. Attach the mode with `=`, as in `--use-mocks=always`; `--use-mocks always` does not select a mode. It affects only Terraform state/output YAML functions; it does not mock Terraform resources or providers.
