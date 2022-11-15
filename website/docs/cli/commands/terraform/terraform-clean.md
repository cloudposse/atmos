---
title: atmos terraform clean
sidebar_label: clean
---

Executes `terraform clean` command.

```shell
atmos terraform clean <component> -s <stack>
```

This command deletes `.terraform` folder, the folder that `TF_DATA_DIR` ENV var points to, `.terraform.lock.hcl` file, `varfile` and `planfile` for a
component in a stack.

:::tip
Run `atmos terraform clean --help` to see all the available options
:::

## Examples

```shell
atmos terraform clean top-level-component1 -s tenant1-ue2-dev
atmos terraform clean infra/vpc -s tenant1-ue2-staging
atmos terraform clean test/test-component -s tenant1-ue2-dev
atmos terraform clean test/test-component-override-2 -s tenant2-ue2-prod
atmos terraform clean test/test-component-override-3 -s tenant1-ue2-dev
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
