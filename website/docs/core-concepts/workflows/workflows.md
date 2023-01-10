---
title: Workflows
sidebar_position: 12
sidebar_label: Workflows
---

Workflows are a way of combining multiple commands into one executable unit of work.

## Simple Example

Here's an example workflow called `eks-up` which runs a few commands that will bring up the EKS cluster:

```yaml
workflows:
  eks-up:
    description: |
      Bring up the EKS cluster.
    steps:
      - command: terraform apply vpc -auto-approve
      - command: terraform apply eks/cluster -auto-approve
      - command: terraform apply eks/alb-controller -auto-approve
```

<br/>

:::note

The workflow name can be anything you want, and the workflows can also be parameterized.

:::

<br/>

If you define this workflow in the file `workflow1.yaml`, it can we executed like this to provision
the `vpc`, `eks/cluster` and `eks/alb-controller` [Atmos Components](/core-concepts/components) into
the `tenant1-ue2-dev` [Atmos Stack](/core-concepts/stacks):

```shell
atmos workflow eks-up -f workflow1 --stack tenant1-ue2-dev
```

<br/>

:::tip

Refer to [`atmos workflow`](/cli/commands/workflow) for the complete description of the CLI command

:::

## Configuration

To configure and execute Atmos workflows, follow these steps:

- Configure workflows in [`atmos.yaml` CLI config file](/cli/configuration)
- Create workflow files and define workflows using the workflow schema

### Configure Workflows in `atmos.yaml`

In `atmos.yaml` CLI config file, add the following sections related to Atmos workflows:

```yaml
# Base path for components, stacks and workflows configurations.
# Can also be set using 'ATMOS_BASE_PATH' ENV var, or '--base-path' command-line argument.
# Supports both absolute and relative paths.
# If not provided or is an empty string, 'components.terraform.base_path', 'components.helmfile.base_path', 'stacks.base_path' 
# and 'workflows.base_path' are independent settings (supporting both absolute and relative paths).
# If 'base_path' is provided, 'components.terraform.base_path', 'components.helmfile.base_path', 'stacks.base_path' 
# and 'workflows.base_path' are considered paths relative to 'base_path'.
base_path: ""

workflows:
  # Can also be set using 'ATMOS_WORKFLOWS_BASE_PATH' ENV var, or '--workflows-dir' command-line arguments
  # Supports both absolute and relative paths
  base_path: "stacks/workflows"
```

where:

- `base_path` - the base path for components, stacks and workflows configurations

- `workflows.base_path` - the base path to Atmos workflow files

### Create Workflow Files and Define Workflows

In `atmos.yaml`, we set `workflows.base_path` to `stacks/workflows`. The folder is relative to the root of the repository.

Refer to [workflow1.yaml](https://github.com/cloudposse/atmos/tree/master/examples/complete/stacks/workflows/workflow1.yaml) for an example.

We put the workflow files into the folder. The workflow file names can be anything you want, but we recommend naming them according to the functions
they perform, e.g. create separate workflow files per environment, account, team, or service.

For example, you can have a workflow file `stacks/workflows/workflows-eks.yaml` to define all EKS-related workflows.

Or, you can have a workflow file `stacks/workflows/workflows-dev.yaml` to define all workflows to provision resources into the `dev` account.
Similarly, you can create a workflow file `stacks/workflows/workflows-prod.yaml` to define all workflows to provision resources into the `prod`
account.

You can segregate the workflow files even further, e.g. per account and service. For example, in the workflow
file `stacks/workflows/workflows-dev-eks.yaml` you can define all EKS-related workflows for the `dev` account.

Workflow files must confirm to the following schema:

```yaml
workflows:

  workflow-1:
    description: "Description of Workflow #1"
    steps: []

  workflow-2:
    description: "Description of Workflow #2"
    steps: []
```

Each workflow file must have the `workflows:` top-level section with a map of workflow definitions.

Each workflow definition must confirm to the following schema:

```yaml
  workflow-1:
    description: "Description of Workflow #1"
    stack: <Atmos stack (optional)>
    steps:
      - command: <Atmos command to execute>
        type: atmos  # optional
        stack: <Atmos stack (optional)>
      - command: <Atmos command to execute>
        stack: <Atmos stack (optional)>
      - command: <shell script>
        type: shell  # required for the steps of type `shell`
```

where:

- `description` - the workflow description

- `stack` - workflow-level Atmos stack (optional). If specified, all workflow steps of type `atmos` will be executed for this Atmos stack. It can be
  overridden in each step or on the command line by using the `--stack` flag (`-s` for shorthand)

- `steps` - a list of workflow steps which are executed sequentially in the order they are specified

Each step is configured using the following attributes:

- `command` - the command to execute. Can be either an Atmos [CLI command](/category/commands-1) (without the `atmos` binary name in front of it,
  e.g. `command: terraform apply vpc`), or a shell script. The type of the command is specified by the `type` attribute

- `type` - the type of the command. Can be either `atmos` or `shell`. Type `atmos` is implicit, you don't have to specify it if the `command`
  is an Atmos [CLI command](/category/commands-1). Type `shell` is required if the command is a shell script. When executing a step of type `atmos`,
  Atmos prepends the `atmos` binary name to the provided command before executing it

- `stack` - step-level Atmos stack (optional). If specified, the `command` will be executed for this Atmos stack. It overrides the
  workflow-level  `stack` attribute, and can itself be overridden on the command line by using the `--stack` flag (`-s` for shorthand)

<br/>

:::note

A workflow command of type `shell` can be any simple or complex shell command or script.
You can use [YAML Multiline Strings](https://yaml-multiline.info/) to create complex multi-line shell scripts.

:::

## Workflow Examples

The following workflow defines four steps of type `atmos` (implicit type) without specifying the workflow-level or step-level `stack` attribute.
Since the workflow does not specify the stack, it's generic and can be executed for any Atmos stack.
In this case, the stack needs to be provided on the command line.

```yaml title=stacks/workflows/workflow1.yaml
workflows:
  terraform-plan-all-test-components:
    description: |
      Run 'terraform plan' on 'test/test-component' and all its derived components.
      The stack must be provided on the command line: 
      `atmos workflow terraform-plan-all-test-components -f workflow1 -s <stack>`
    steps:
      # Inline scripts are also supported
      # Refer to https://yaml-multiline.info for more details
      - type: shell
        command: >-
          echo "Starting the workflow execution..."
          read -p "Press any key to continue... " -n1 -s
      - command: terraform plan test/test-component
      - command: terraform plan test/test-component-override
      - command: terraform plan test/test-component-override-2
      - command: terraform plan test/test-component-override-3
      - type: shell
        command: >-
          echo "All done!"
```

To run this workflow for the `tenant1-ue2-dev` stack, execute the following command:

```console
atmos workflow terraform-plan-all-test-components -f workflow1 -s tenant1-ue2-dev
```

<br/>

The following workflow executes `terraform plan` on `test/test-component-override-2` component in all stacks.
In this case, the stack is specified inline for each workflow command.

```yaml title=stacks/workflows/workflow1.yaml
workflows:
  terraform-plan-test-component-override-2-all-stacks:
    description: Run 'terraform plan' on 'test/test-component-override-2' component in all stacks
    steps:
      - command: terraform plan test/test-component-override-2 -s tenant1-ue2-dev
      - command: terraform plan test/test-component-override-2 -s tenant1-ue2-staging
      - command: terraform plan test/test-component-override-2 -s tenant1-ue2-prod
      - command: terraform plan test/test-component-override-2 -s tenant2-ue2-dev
      - command: terraform plan test/test-component-override-2 -s tenant2-ue2-staging
      - command: terraform plan test/test-component-override-2 -s tenant2-ue2-prod
      - type: shell
        command: echo "All done!"
```

To run this workflow, execute the following command:

```console
atmos workflow terraform-plan-test-component-override-2-all-stacks -f workflow1
```

<br/>

The following workflow is similar to the above, but the stack for each command is specified in the step-level `stack` attribute.

```yaml title=stacks/workflows/workflow1.yaml
workflows:
  terraform-plan-test-component-override-3-all-stacks:
    description: Run 'terraform plan' on 'test/test-component-override-3' component in all stacks
    steps:
      - command: terraform plan test/test-component-override-3
        stack: tenant1-ue2-dev
      - command: terraform plan test/test-component-override-3
        stack: tenant1-ue2-staging
      - command: terraform plan test/test-component-override-3
        stack: tenant1-ue2-prod
      - command: terraform plan test/test-component-override-3
        stack: tenant2-ue2-dev
      - command: terraform plan test/test-component-override-3
        stack: tenant2-ue2-staging
      - command: terraform plan test/test-component-override-3
        stack: tenant2-ue2-prod
```

To run this workflow, execute the following command:

```console
atmos workflow terraform-plan-test-component-override-3-all-stacks -f workflow1
```

<br/>

__Note__ that the stack for the commands of type `atmos` can be specified in four different ways:

- Inline in the command itself
  ```yaml
  steps:
    - command: terraform plan test/test-component-override-2 -s tenant1-ue2-dev
  ```

- In the workflow-level `stack` attribute
  ```yaml
  workflows:
    my-workflow:
    stack: tenant1-ue2-dev
    steps:
      - command: terraform plan test/test-component
  ```

- In the step-level `stack` attribute
  ```yaml
  steps:
    - command: terraform plan test/test-component
      stack: tenant1-ue2-dev
  ```

- On the command line
  ```console
  atmos workflow my-workflow -f workflow1 -s tenant1-ue2-dev
  ```

<br/>

The stack defined inline in the command itself has the lowest priority, it can and will be overridden by any other stack definition.
The step-level stack will override the workflow-level stack. The command line `--stack` option will override all other stacks defined in the workflow
itself. You can also use any combinations of the above (e.g. specify the stack at the workflow level, then override it at the step level for some
commands, etc.).

While this provides a great flexibility in defining the stack for workflow commands, we recommend creating generic workflows without defining
stacks in the workflow itself (the stack should be provided on the command line). This way, the workflow can be executed for any stack without any
modifications and without dealing with multiple workflows that are similar but differ only by the environment where the resources are provisioned.
