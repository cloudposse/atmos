---
title: atmos atlantis generate repo-config
sidebar_label: generate repo-config
sidebar_class_name: command
id: generate-repo-config
description: Use this command to generate repository configuration for Atlantis.
---

:::info Purpose
Use this command to generate repository configuration for Atlantis.
:::

```shell
atmos atmos atlantis generate repo-config [options]
```

:::tip
Run `atmos atlantis generate repo-config --help` to see all the available options
:::

## Examples

```shell
atmos atlantis generate repo-config --config-template config-1 --project-template project-1 --workflow-template workflow-1

atmos atlantis generate repo-config --config-template config-1 --project-template project-1 --workflow-template workflow-1 --stacks <stack1, stack2>

atmos atlantis generate repo-config --config-template config-1 --project-template project-1 --workflow-template workflow-1 --components <component1, component2>

atmos atlantis generate repo-config --config-template config-1 --project-template project-1 --workflow-template workflow-1 --stacks <stack1> --components <component1, component2>
```

## Flags

| Flag                  | Description                                                                           | Required |
|:----------------------|:--------------------------------------------------------------------------------------|:---------|
| `--config-template`   | Atlantis config template name                                                         | yes      |
| `--project-template`  | Atlantis project template name                                                        | yes      |
| `--workflow-template` | Atlantis workflow template name                                                       | yes      |
| `--output-path`       | Output path to write `atlantis.yaml` file                                             | no       |
| `--stacks`            | Generate Atlantis projects for the specified stacks only (comma-separated values)     | no       |
| `--components`        | Generate Atlantis projects for the specified components only (comma-separated values) | no       |

<br/>

:::info
For more information, refer to:

- [Repo Level atlantis.yaml Config](https://www.runatlantis.io/docs/repo-level-atlantis-yaml.html)
- [Atlantis Custom Workflows](https://www.runatlantis.io/docs/custom-workflows.html)
  :::
