---
title: atmos atlantis generate repo-config
sidebar_label: generate repo-config
sidebar_class_name: command
id: generate-repo-config
description: Use this command to generate a repository configuration for Atlantis.
---

:::info Purpose
Use this command to generate a repository configuration for Atlantis.
:::

<br/>

```shell
atmos atmos atlantis generate repo-config [options]
```

<br/>

:::tip
Run `atmos atlantis generate repo-config --help` to see all the available options
:::

## Examples

```shell
atmos atlantis generate repo-config --config-template config-1 --project-template project-1

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
| `--workflow-template` | Atlantis workflow template name                                                       | no       |
| `--output-path`       | Output path to write `atlantis.yaml` file                                             | no       |
| `--stacks`            | Generate Atlantis projects for the specified stacks only (comma-separated values)     | no       |
| `--components`        | Generate Atlantis projects for the specified components only (comma-separated values) | no       |

## Atlantis Workflows

The flag `--workflow-template` is optional because Atlantis workflows can be specified in two different ways:

- In [Server Side Config](https://www.runatlantis.io/docs/server-side-repo-config.html) using the `workflow` attribute

  ```yaml title=server.yaml
  repos:
    - id: /.*/
      branch: /.*/

      # 'workflow' sets the workflow for all repos that match.
      # This workflow must be defined in the workflows section.
      workflow: custom
  
      # allowed_overrides specifies which keys can be overridden by this repo in
      # its atlantis.yaml file.
      allowed_overrides: [apply_requirements, workflow, delete_source_branch_on_merge, repo_locking]
  
      # allowed_workflows specifies which workflows the repos that match
      # are allowed to select.
      allowed_workflows: [custom]
  
      # allow_custom_workflows defines whether this repo can define its own
      # workflows. If false (default), the repo can only use server-side defined
      # workflows.
      allow_custom_workflows: true  

  # workflows lists server-side custom workflows
  workflows:
    custom:
      plan:
        steps:
          - init
          - plan:
      apply:
        steps:
          - run: echo applying
          - apply  
  ```

- In [Repo Level atlantis.yaml Config](https://www.runatlantis.io/docs/repo-level-atlantis-yaml.html) using the `workflows` section and the `workflow`
  attribute in each Atlantis project in `atlantis.yaml`

  ```yaml title=atlantis.yaml
  version: 3
  projects:
    - name: my-project-name
      branch: /main/
      dir: .
      workspace: default
      workflow: myworkflow
  workflows:
    myworkflow:
      plan:
        steps:
          - init
          - plan
      apply:
        steps:
          - run: echo applying
          - apply
  ```

<br/>

If you use [Server Side Config](https://www.runatlantis.io/docs/server-side-repo-config.html) to define Atlantis workflows,
you don't need to specify the `workflow_templates` section in the [Atlantis Integration](/cli/configuration#integrations) in `atmos.yaml`, and
you don't have to provide the workflow template using the `--workflow-template` flag when executing an `atmos atmos atlantis generate repo-config`
command.

On the other hand, if you define and use workflows
in [Repo Level atlantis.yaml Config](https://www.runatlantis.io/docs/repo-level-atlantis-yaml.html),
workflow templates (at least one) must be provided in the [Atlantis Integration](/cli/configuration#integrations) in the `workflow_templates` section,
and you select one of the templates by using the `--workflow-template` flag.

<br/>

:::info

For more information, refer to:

- [Configuring Atlantis](https://www.runatlantis.io/docs/configuring-atlantis.html)
- [Server Side Config](https://www.runatlantis.io/docs/server-side-repo-config.html)
- [Repo Level atlantis.yaml Config](https://www.runatlantis.io/docs/repo-level-atlantis-yaml.html)
- [Server Configuration](https://www.runatlantis.io/docs/server-configuration.html)
- [Atlantis Custom Workflows](https://www.runatlantis.io/docs/custom-workflows.html)

:::
