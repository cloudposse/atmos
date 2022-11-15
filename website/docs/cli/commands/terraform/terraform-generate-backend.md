---
title: atmos terraform generate backend
sidebar_label: generate backend
---

Executes `terraform generate backend` command.

```shell
atmos terraform generate backend <command> <component> -s <stack>
```

This command generates a backend config file for an `atmos` terraform component in a stack.

:::tip
Run `atmos terraform generate backend --help` to see all the available options
:::

## Examples

```shell
atmos terraform generate backend top-level-component1 -s tenant1-ue2-dev
atmos terraform generate backend infra/vpc -s tenant1-ue2-staging
atmos terraform generate backend test/test-component -s tenant1-ue2-dev
atmos terraform generate backend test/test-component-override-2 -s tenant2-ue2-prod
```

## Arguments

| Argument     | Description                 | Required |
|:-------------|:----------------------------|:---------|
| `component`  | `atmos` terraform component | yes      |

## Flags

| Flag        | Description   | Alias | Required |
|:------------|:--------------|:------|:---------|
| `--stack`   | `atmos` stack | `-s`  | yes      |
| `--dry-run` | Dry-run       |       | no       |

<br/>

:::info
Refer to [Terraform backend configuration](https://developer.hashicorp.com/terraform/language/settings/backends/configuration) for more details
on `terraform` backends and supported formats
:::
