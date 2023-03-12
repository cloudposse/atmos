---
title: atmos describe dependants
sidebar_label: dependants
sidebar_class_name: command
id: dependants
description: This command produces a list of Atmos components in Atmos stacks that depend on the provided Atmos component.
---

:::note Purpose
Use this command to show a list of Atmos components in Atmos stacks that depend on the provided Atmos component.
:::

## Description


## Usage

```shell
atmos describe dependants [options]
```

<br/>

:::tip
Run `atmos describe dependants --help` to see all the available options
:::

## Examples

```shell
atmos describe dependants test/test-component -s tenant1-ue2-test-1
atmos describe dependants test/test-component -s tenant1-ue2-dev --format yaml
atmos describe dependants test/test-component -s tenant1-ue2-test-1 -f yaml
atmos describe dependants test/test-component -s tenant1-ue2-test-1 --file dependants.json
atmos describe dependants test/test-component -s tenant1-ue2-test-1 --format yaml --file dependants.yaml
```

## Arguments

| Argument    | Description     | Required |
|:------------|:----------------|:---------|
| `component` | Atmos component | yes      |

## Flags

| Flag       | Description                                         | Alias | Required |
|:-----------|:----------------------------------------------------|:------|:---------|
| `--stack`  | Atmos stack                                         | `-s`  | yes      |
| `--format` | Output format: `json` or `yaml` (`json` is default) | `-f`  | no       |
| `--file`   | If specified, write the result to the file          |       | no       |

## Output

The command outputs a list of objects (in JSON or YAML format).

Each object has the following schema:

```json
{
  "component": "....",
  "component_type": "....",
  "component_path": "....",
  "namespace": "....",
  "tenant": "....",
  "environment": "....",
  "stage": "....",
  "stack": "....",
  "spacelift_stack": ".....",
  "atlantis_project": "....."
}
```

where:

- `component` - the dependant Atmos component in the stack

- `component_type` - the type of the dependant component (`terraform` or `helmfile`)

- `component_path` - the filesystem path to the `terraform` or `helmfile` component

- `namespace` - the `namespace` where the dependant Atmos component is provisioned

- `tenant` - the `tenant` where the dependant Atmos component is provisioned

- `environment` - the `environment` where the dependant Atmos component is provisioned

- `stage` - the `stage` where the dependant Atmos component is provisioned

- `stack` - the Atmos stack where the dependant Atmos component is provisioned

- `spacelift_stack` - the dependant Spacelift stack. It will be included only if the Spacelift workspace is enabled for the dependant Atmos component
  in the Atmos stack in the `settings.spacelift.workspace_enabled` section (either directly in the component's `settings.spacelift.workspace_enabled`
  section or via inheritance)

- `atlantis_project` - the dependant Atlantis project name. It will be included only if the Atlantis integration is configured in
  the `settings.atlantis` section in the stack config. Refer to [Atlantis Integration](/integrations/atlantis.md) for more details

<br/>

:::note

Abstract Atmos components (`metadata.type` is set to `abstract`) are not included in the output since they serve as blueprints for other
Atmos components and are not meant to be provisioned.

:::

## Output Example

```shell
atmos describe dependants test/test-component -s tenant1-ue2-test-1
```

```json
[
  {
    "component": "top-level-component2",
    "component_type": "terraform",
    "component_path": "examples/complete/components/terraform/top-level-component1",
    "namespace": "cp",
    "tenant": "tenant1",
    "environment": "ue2",
    "stage": "test-1",
    "stack": "tenant1-ue2-test-1",
    "atlantis_project": "tenant1-ue2-test-1-top-level-component2"
  },
  {
    "component": "top-level-component1",
    "component_type": "terraform",
    "component_path": "examples/complete/components/terraform/top-level-component1",
    "namespace": "cp",
    "tenant": "tenant1",
    "environment": "ue2",
    "stage": "dev",
    "stack": "tenant1-ue2-dev",
    "spacelift_stack": "tenant1-ue2-dev-top-level-component1",
    "atlantis_project": "tenant1-ue2-dev-top-level-component1"
  }
]
```
