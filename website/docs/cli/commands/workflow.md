---
title: atmos workflow
sidebar_label: workflow
---

An Atmos Workflow is a series of steps that are run in order to achieve some outcome. Every workflow has a name and is easily executed from the
command line by calling `atmos workflow`. Use workflows to orchestrate a any number of commands. Workflows can call any `atmos` subcommand, shell
commands, and has access to the stack configurations.

## Executes `workflow` command

```shell
atmos workflow [options]
```

Allows sequential execution of `atmos` and `shell` commands defined as workflow steps.

:::tip
Run `atmos workflow --help` to see all the available options
:::

### Examples

```shell
atmos workflow test-1 -f workflow1
atmos workflow terraform-plan-all-test-components -f workflow1 -s tenant1-ue2-dev
atmos workflow terraform-plan-test-component-override-2-all-stacks -f workflow1 --dry-run
atmos workflow terraform-plan-all-tenant1-ue2-dev -f workflow1
```
