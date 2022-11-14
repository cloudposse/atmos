---
title: atmos describe component
sidebar_label: describe component
---

Executes `describe component` command.

```shell
atmos describe component <component> -s <stack>
```

This command shows configuration for a component in a stack.

:::tip
Run `atmos describe component --help` to see all the available options
:::

## Examples

```shell
atmos describe component infra/vpc -s tenant1-ue2-dev
atmos describe component echo-server -s tenant1-ue2-staging
atmos describe component test/test-component-override -s tenant2-ue2-prod
```

## Arguments

| Argument     | Description        | Required |
|:-------------|:-------------------|:---------|
| `component`  | `atmos` component  | yes      |

## Flags

| Flag        | Description   | Alias | Required |
|:------------|:--------------|:------|:---------|
| `--stack`   | `atmos` stack | `-s`  | yes      |
