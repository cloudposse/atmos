---
title: Workflows
sidebar_position: 12
sidebar_label: Workflows
---

Workflows are a way of combining multiple commands into one executable unit of work.

## Simple Example

Here's an example workflow called `eks-up` which runs a few commands that will bring up the cluster.
The workflow name can be anything you want, and the workflows can also be parameterized.

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

Then run this workflow like this:

```shell
atmos workflow eks-up --stack tenant1-ue2-dev
```

## Configuration

Workflows can be defined using two different methods:

- **Stack configurations:** Add the `workflows` section to any Stack configuration.
- **Standalone files:**  Add reusable `workflows` in a separate file (
  see [workflow1.yaml](https://github.com/cloudposse/atmos/tree/master/examples/complete/stacks/workflows/workflow1.yaml)

In the first case, we define workflows in the Stack configuration file (which we specify on the command line).
To execute the workflows for some stack (e.g. `tenant1-ue2-dev`), run the following commands:

```shell
atmos workflow eks-up -s tenant1-ue2-dev
```

**Note:** Workflows defined in the stack config files can be executed only for the particular stack (environment and stage). It's not possible to
provision resources for multiple stacks this way.

In the second case (defining workflows in a separate file), a single workflow can be created to provision resources into different stacks. The stacks
for the workflow steps can be specified in the workflow config.

## Using Workflow Files

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
