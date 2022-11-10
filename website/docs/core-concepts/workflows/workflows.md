---
sidebar_position: 12
title: Workflows
---

# Workflows


Workflows are a way of combining multiple commands into one executable unit of work.

In the CLI, workflows can be defined using two different methods:

- In the configuration file for a stack (see [workflows in ue2-dev.yaml](example/stacks/ue2-dev.yaml) for an example)
- In a separate file (see [workflow1.yaml](example/stacks/workflows/workflow1.yaml)

In the first case, we define workflows in the configuration file for the stack (which we specify on the command line).
To execute the workflows from [workflows in ue2-dev.yaml](example/stacks/ue2-dev.yaml), run the following commands:

```console
atmos workflow terraform-plan-all-tenant1-ue2-dev -s tenant1-ue2-dev
```

Note that workflows defined in the stack config files can be executed only for the particular stack (environment and stage).
It's not possible to provision resources for multiple stacks this way.

In the second case (defining workflows in a separate file), a single workflow can be created to provision resources into different stacks.
The stacks for the workflow steps can be specified in the workflow config.

For example, to run `terraform plan` and `helmfile diff` on all terraform and helmfile components in the example, execute the following command:

```console
atmos workflow plan-all -f workflows
```

where the command-line option `-f` (`--file` for long version) instructs the `atmos` CLI to look for the `plan-all` workflow in the file [workflows](example/stacks/workflows.yaml).

As we can see, in multi-environment workflows, each workflow job specifies the stack it's operating on:

```yaml
workflows:
terraform-plan-all-test-components:
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

For example, to run the `terraform-plan-all-tenant1-ue2-dev` workflow from the [workflows](example/stacks/workflows/workflow1.yaml) file for the `tenant1-ue2-dev` stack,
execute the following command:

```console
atmos workflow terraform-plan-all-tenant1-ue2-dev -f workflow1 -s tenant1-ue2-dev
```
