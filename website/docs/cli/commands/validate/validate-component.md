---
title: atmos validate component
sidebar_label: component
sidebar_class_name: command
id: component
description: Use this command to validate an Atmos component in a stack using JSON Schema and OPA policies.
---

:::note purpose
Use this command to validate an Atmos component in a stack using JSON Schema and OPA policies.
:::

## Usage

Execute the `validate component` command like this:

```shell
atmos validate component <component> -s <stack> [options]
```

This command validates an `atmos` component in a stack using JSON Schema and OPA policies.

<br/>

:::tip
Run `atmos validate component --help` to see all the available options
:::

## Examples

```shell
atmos validate component infra/vpc -s tenant1-ue2-dev
atmos validate component infra/vpc -s tenant1-ue2-dev --schema-path validate-infra-vpc-component.json --schema-type jsonschema
atmos validate component infra/vpc -s tenant1-ue2-dev --schema-path validate-infra-vpc-component.rego --schema-type opa
```

## Arguments

| Argument     | Description        | Required |
|:-------------|:-------------------|:---------|
| `component`  | `atmos` component  | yes      |

## Flags

| Flag            | Description                                                                                                                                                       | Alias | Required |
|:----------------|:------------------------------------------------------------------------------------------------------------------------------------------------------------------|:------|:---------|
| `--stack`       | `atmos` stack                                                                                                                                                     | `-s`  | yes      |
| `--schema-path` | Path to the schema file.<br/>Can be an absolute path or a path relative to `schemas.jsonschema.base_path`<br/>and `schemas.opa.base_path` defined in `atmos.yaml` |       | no       |
| `--schema-type` | Schema type: `jsonschema` or `opa`                                                                                                                                |       | no       |
