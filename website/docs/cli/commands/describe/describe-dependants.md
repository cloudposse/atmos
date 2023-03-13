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

In Atmos, you can define stack and component dependencies by using the `settings.dependencies` section.

The `settings.dependencies.depends_on` subsection is used to define all the Atmos components (in the same or different stacks) that the current
component depends on.

:::note

The `depends_on` subsection is used to define Atmos component dependencies instead of the `settings.dependencies` section because
the `settings.dependencies` section is a parent section for all types of stack and component dependencies. In the future we could add other
functionality to define various types of component and stack dependencies by using different subsections in the `settings.dependencies` section.

:::

<br/>

In the following example we specify that the `top-level-component1` Atmos component depends on the following:

- The `test/test-component-override` component in the same Atmos stack
- The `test/test-component` component in an Atmos stack identified by the `dev` stage

```yaml title="examples/complete/stacks/catalog/terraform/top-level-component1.yaml"
components:
  terraform:
    top-level-component1:
      settings:
        dependencies:
          depends_on:
            "test/test-component-override":
              # If the `context` (namespace, tenant, environment, stage) is not provided, the `component` is from the same stack as this component
              component: "test/test-component-override"
            "dev-test/test-component":
              # This component (in any stage) always depends on `test/test-component` from the `dev` stage
              component: "test/test-component"
              stage: "dev"
      vars:
        enabled: true
```

In the following example we specify that the `top-level-component2` Atmos component depends on the following:

- The `test/test-component` component in the same Atmos stack
- The `test/test2/test-component-2` in the same Atmos stack

```yaml title="examples/complete/stacks/catalog/terraform/top-level-component2.yaml"
components:
  terraform:
    top-level-component2:
      metadata:
        component: "top-level-component1"
      settings:
        dependencies:
          depends_on:
            "test/test-component":
              # If the `context` (namespace, tenant, environment, stage) is not provided, the `component` is from the same stack as this component
              component: "test/test-component"
            "test/test2/test-component-2":
              # If the `context` (namespace, tenant, environment, stage) is not provided, the `component` is from the same stack as this component
              component: "test/test2/test-component-2"
      vars:
        enabled: true
```

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
