---
title: atmos terraform workspace
sidebar_label: terraform workspace
---

Executes `terraform workspace` command.

```shell
atmos terraform workspace <component> -s <stack>
```

This command calculates the `terraform` workspace for an `atmos` component (from the context variables) and then selects it by
executing the `terraform workspace select` command.

If the workspace does not exist, the command creates it by executing the `terraform workspace new` command.

<br/>

:::tip
Run `atmos terraform workspace --help` to see all the available options
:::

## Examples

```shell
atmos terraform workspace top-level-component1 -s tenant1-ue2-dev
atmos terraform workspace infra/vpc -s tenant1-ue2-staging
atmos terraform workspace test/test-component -s tenant1-ue2-dev
atmos terraform workspace test/test-component-override-2 -s tenant2-ue2-prod
atmos terraform workspace test/test-component-override-3 -s tenant1-ue2-dev
```

## Arguments

| Argument     | Description        | Required |
|:-------------|:-------------------|:---------|
| `component`  | `atmos` component  | yes      |

## Flags

| Flag        | Description   | Alias | Required |
|:------------|:--------------|:------|:---------|
| `--stack`   | `atmos` stack | `-s`  | yes      |
