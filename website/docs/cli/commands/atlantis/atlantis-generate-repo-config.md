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

atmos atlantis generate repo-config --config-template config-1 --project-template project-1

atmos atlantis generate repo-config --config-template config-1 --project-template project-1 --stacks <stack1, stack2>

atmos atlantis generate repo-config --config-template config-1 --project-template project-1 --components <component1, component2>

atmos atlantis generate repo-config --config-template config-1 --project-template project-1 --stacks <stack1> --components <component1, component2>
```

## Flags

| Flag                  | Description                                                                           | Required |
|:----------------------|:--------------------------------------------------------------------------------------|:---------|
| `--config-template`   | Atlantis config template name                                                         | yes      |
| `--project-template`  | Atlantis project template name                                                        | yes      |
| `--output-path`       | Output path to write `atlantis.yaml` file                                             | no       |
| `--stacks`            | Generate Atlantis projects for the specified stacks only (comma-separated values)     | no       |
| `--components`        | Generate Atlantis projects for the specified components only (comma-separated values) | no       |

## Atlantis Workflows

- In [Server Side Config](https://www.runatlantis.io/docs/server-side-repo-config.html) using the `workflows` section and `workflow` attribute

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
          - plan
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
you don't need to specify the `workflow_templates` section in the [Atlantis Integration](/cli/configuration#integrations) section in `atmos.yaml`
when executing an `atmos atlantis generate repo-config`command. After you defined the workflows in the server config `workflows` section, 
you can reference a workflow to be used for each generated Atlantis project in the `integrations.atlantis.project_templates` section, for example:

```yaml title=atmos.yaml
integrations:

  # Atlantis integration
  atlantis:
    path: "atlantis.yaml"

    # Project templates
    # Select a template by using the '--project-template <project_template>' command-line argument in 'atmos atlantis generate repo-config' command
    project_templates:
      project-1:
        name: "{tenant}-{environment}-{stage}-{component}"
        workflow: custom
```

On the other hand, if you define and use workflows
in [Repo Level atlantis.yaml Config](https://www.runatlantis.io/docs/repo-level-atlantis-yaml.html),
you need to provide at least one workflow template in the `workflow_templates` section in [Atlantis Integration](/cli/configuration#integrations).

For example, after executing the following command:

```console
atmos atlantis generate repo-config --config-template config-1 --project-template project-1
```

the `atlantis.yaml` file would look like this:

```yaml
version: 3
projects:
  - name: tenant1-ue2-dev-infra-vpc
    workspace: tenant1-ue2-dev
    workflow: workflow-1

workflows:
  workflow-1:
    apply:
      steps:
        - run: terraform apply $PLANFILE
    plan:
      steps:
        - run: terraform init -input=false
        - run: terraform workspace select $WORKSPACE || terraform workspace new $WORKSPACE
        - run: terraform plan -input=false -refresh -out $PLANFILE -var-file varfiles/$PROJECT_NAME.tfvars.json
```

<br/>

:::info

For more information, refer to:

- [Configuring Atlantis](https://www.runatlantis.io/docs/configuring-atlantis.html)
- [Server Side Config](https://www.runatlantis.io/docs/server-side-repo-config.html)
- [Repo Level atlantis.yaml Config](https://www.runatlantis.io/docs/repo-level-atlantis-yaml.html)
- [Server Configuration](https://www.runatlantis.io/docs/server-configuration.html)
- [Atlantis Custom Workflows](https://www.runatlantis.io/docs/custom-workflows.html)

:::
