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

This command validates an Atmos component in a stack using JSON Schema and OPA policies.

<br/>

:::tip
Run `atmos validate component --help` to see all the available options
:::

## Examples

```shell
atmos validate component infra/vpc -s tenant1-ue2-dev
atmos validate component infra/vpc -s tenant1-ue2-dev --schema-path vpc/validate-infra-vpc-component.json --schema-type jsonschema
atmos validate component infra/vpc -s tenant1-ue2-dev --schema-path vpc/validate-infra-vpc-component.rego --schema-type opa
atmos validate component infra/vpc -s tenant1-ue2-dev --schema-path vpc/validate-infra-vpc-component.rego --schema-type opa --module-paths constants
atmos validate component infra/vpc -s tenant1-ue2-dev --timeout 15
```

## Arguments

| Argument    | Description     | Required |
|:------------|:----------------|:---------|
| `component` | Atmos component | yes      |

## Flags

| Flag             | Description                                                                                                                                                                                            | Alias | Required |
|:-----------------|:-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|:------|:---------|
| `--stack`        | Atmos stack                                                                                                                                                                                            | `-s`  | yes      |
| `--schema-path`  | Path to the schema file.<br/>Can be an absolute path or a path relative to `schemas.jsonschema.base_path`<br/>and `schemas.opa.base_path` defined in `atmos.yaml`                                      |       | no       |
| `--schema-type`  | Schema type: `jsonschema` or `opa`                                                                                                                                                                     |       | no       |
| `--module-paths` | Comma-separated string of filesystem paths to the additional modules for schema validation<br/>Each path can be an absolute path or a path relative to `schemas.opa.base_path` defined in `atmos.yaml` |       | no       |
| `--timeout`      | Validation timeout in seconds. Can also be specified in `settings.validation` component config. If not provided, timeout of 20 seconds is used by default                                              |       | no       |
