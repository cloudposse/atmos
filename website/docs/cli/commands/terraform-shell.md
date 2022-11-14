---
title: atmos terraform shell
sidebar_label: terraform shell
---

Executes `terraform shell` command.

```shell
atmos terraform shell <component> -s <stack>
```

The command configures an environment for an `atmos` component in a stack and starts a new shell allowing executing all native terraform commands
inside the shell without using atmos-specific arguments and flags.

The command does the following:

- Processes the stack config files, generates the required variables for the `atmos` component in the stack, and writes them to a file in the
  component's folder

- Generates a backend config file for the `atmos` component in the stack and writes it to a file in the component's folder

- Creates a `terraform` workspace for the component in the stack

- Drops the user into a separate shell (process) with all the required paths and ENV vars set

- Inside the shell, the user can execute all `terraform` commands using the native syntax

<br/>

:::tip
Run `atmos terraform shell --help` to see all the available options
:::

## Examples

```shell
atmos terraform shell top-level-component1 -s tenant1-ue2-dev
atmos terraform shell infra/vpc -s tenant1-ue2-staging
atmos terraform shell test/test-component-override-3 -s tenant2-ue2-prod
```

## Arguments

| Argument     | Description        | Required |
|:-------------|:-------------------|:---------|
| `component`  | `atmos` component  | yes      |

## Flags

| Flag        | Description   | Alias | Required |
|:------------|:--------------|:------|:---------|
| `--stack`   | `atmos` stack | `-s`  | yes      |
