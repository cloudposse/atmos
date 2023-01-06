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
      - command: terraform apply vpc
      - command: terraform apply eks/cluster
      - command: terraform apply eks/alb-controller
```

<br/>

:::note

The workflow name can be anything you want, and the workflows can also be parameterized.

:::

<br/>

If you define this workflow in the file `workflow1.yaml`, it can we executed like this:

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
- Create workflow files
- Define workflows using workflow schema

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

- `base_path` - the base path for components, stacks and workflows configurations

- `workflows.base_path` - the base path to Atmos workflow files

### Create Workflow Files

In `atmos.yaml`, we set `workflows.base_path` to `stacks/workflows`. The folder is relative to the root of the repository.

Refer to [workflow1.yaml](https://github.com/cloudposse/atmos/tree/master/examples/complete/stacks/workflows/workflow1.yaml) as an example.

We put the workflow files into the folder. The workflow file names can be anything you want, but we recommend naming them according to the functions
they are performing, e.g. create separate workflow files per environment, account, team, or service.

For example, you can have a workflow file `stacks/workflows/workflows-eks.yaml` to define all EKS-related workflows.

Or, you can have a workflow file `stacks/workflows/workflows-dev.yaml` to define all workflows to provision resources into the `dev` account.
Similarly, you can create a workflow file `stacks/workflows/workflows-prod.yaml` to define all workflows to provision resources into the `prod`
account.

You can segregate the workflow files even further per account and service. For example, in the workflow
file `stacks/workflows/workflows-dev-eks.yaml` you can define all EKS-related workflows for the `dev` account.

Workflow files must confirm to the following schema:

```yaml
workflows:

  workflow-1:
    description: "Description of Workflow #1"
    steps: [ ]

  workflow-2:
    description: ""
    steps: [ ]
```

Each file must have the `workflows:` top-level section with a map of workflow definitions.

### Workflow Schema

Each workflow definition must confirm to the following schema:

```yaml
workflows:

  workflow-1:
    description: "Description of Workflow #1"
    steps: [ ]

  workflow-2:
    description: "Description of Workflow #2"
    steps: [ ]
```

:::note

A workflow command of type `shell` can be any simple or complex shell command or script

:::

## Workflow Examples

For example, to run `terraform plan` and `helmfile diff` on all terraform and helmfile components in the example, execute the following command:

```console
atmos workflow plan-all -f workflows
```

where the command-line option `-f` (`--file` for long version) instructs the `atmos` CLI to look for the `plan-all` workflow in the
file [workflows](https://github.com/cloudposse/atmos/tree/master/examples/complete/stacks/workflows/workflow1.yaml).

As we can see, in multi-environment workflows, each workflow job specifies the stack it's operating on:

```yaml
workflows:
  plan-all:
    description: |
      Run 'terraform plan' on 'test/test-component' and all its derived components.
      The stack must be provided on the command line: atmos workflow terraform-plan-all-test-components -f workflow1 -s <stack>
    steps:
      - command: terraform plan test/test-component
      - command: terraform plan test/test-component-override
      - command: terraform plan test/test-component-override-2
      - command: terraform plan test/test-component-override-3
```

You can also define a workflow in a separate file without specifying the stack in the workflow's job config.
In this case, the stack needs to be provided on the command line.

For example, to run the `plan-all` workflow from
the [workflows](https://github.com/cloudposse/atmos/tree/master/example/stacks/workflows/workflow1.yaml) file for the `tenant1-ue2-dev` stack,
execute the following command:

```console
atmos workflow plan-all -f workflow1 -s tenant1-ue2-dev
```
