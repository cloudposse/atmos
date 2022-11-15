---
title: atmos terraform generate varfile
sidebar_label: generate varfile
sidebar_class_name: command
---

Executes `terraform generate varfile` command.

```shell
atmos terraform generate varfile <command> <component> -s <stack>
```

This command generates a varfile for an `atmos` terraform component in a stack.

:::tip
Run `atmos terraform generate varfile --help` to see all the available options
:::

## Examples

```shell
atmos terraform generate varfile top-level-component1 -s tenant1-ue2-dev
atmos terraform generate varfile infra/vpc -s tenant1-ue2-staging
atmos terraform generate varfile test/test-component -s tenant1-ue2-dev
atmos terraform generate varfile test/test-component-override-2 -s tenant2-ue2-prod
atmos terraform generate varfile test/test-component-override-3 -s tenant1-ue2-dev -f vars.json
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
