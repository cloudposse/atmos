---
title: atmos helmfile generate varfile
sidebar_label: generate varfile
sidebar_class_name: command
id: generate-varfile
description: Use this command to generate a varfile for a `helmfile` component in a stack.
---

:::note Purpose
Use this command to generate a varfile for a `helmfile` component in a stack.
:::

## Usage

Execute the `helmfile generate varfile` command like this:

```shell
atmos helmfile generate varfile <component> -s <stack> [options]
```

This command generates a varfile for a `helmfile` component in a stack.

:::tip
Run `atmos helmfile generate varfile --help` to see all the available options
:::

## Examples

```shell
atmos helmfile generate varfile echo-server -s tenant1-ue2-dev
atmos helmfile generate varfile echo-server -s tenant1-ue2-dev
atmos helmfile generate varfile echo-server -s tenant1-ue2-dev -f vars.yaml
atmos helmfile generate varfile echo-server --stack tenant1-ue2-dev --file=vars.yaml
```

## Arguments

| Argument    | Description                | Required |
|:------------|:---------------------------|:---------|
| `component` | `atmos` helmfile component | yes      |

## Flags

| Flag        | Description                                                                                                           | Alias | Required |
|:------------|:----------------------------------------------------------------------------------------------------------------------|:------|:---------|
| `--stack`   | `atmos` stack                                                                                                         | `-s`  | yes      |
| `--file`    | File name to write the varfile to.<br/>If not specified, the varfile name is generated automatically from the context | `-f`  | no       |
| `--dry-run` | Dry-run                                                                                                               |       | no       |
