---
title: atmos workflow
sidebar_label: workflow
sidebar_class_name: command
description: Use this command to perform sequential execution of `atmos` and `shell` commands defined as workflow steps.
---

:::note Purpose
Use this command to perform sequential execution of `atmos` and `shell` commands defined as workflow steps.
:::

## Usage

Execute the `terraform workflow` command like this:

```shell
atmos workflow <workflow_name> --file <workflow_file> [options]
```

This command allows sequential execution of `atmos` and `shell` commands defined as workflow steps.

An Atmos workflow is a series of steps that are run in order to achieve some outcome. Every workflow has a name and is easily executed from the
command line by calling `atmos workflow`. Use workflows to orchestrate any number of commands. Workflows can call any `atmos` subcommand, shell
commands, and has access to the stack configurations.

<br/>

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

## Arguments

| Argument         | Description   | Required |
|:-----------------|:--------------|:---------|
| `workflow_name ` | Workflow name | yes      |

## Flags

| Flag        | Description                                                                                   | Alias | Required |
|:------------|:----------------------------------------------------------------------------------------------|:------|:---------|
| `--file`    | File name where the workflow is defined                                                       | `-f`  | yes      |
| `--stack`   | Atmos stack<br/>(if provided, will override stacks defined in the workflow or workflow steps) | `-s`  | no       |
| `--dry-run` | Dry run                                                                                       |       | no       |
