---
title: atmos terraform
sidebar_label: terraform
---

Executes `terraform` commands.

```shell
atmos terraform <command> <component> -s <stack> [options]
atmos terraform <command> <component> --stack <stack> [options]
```

<br/>

:::info
`atmos` supports all `terraform` commands and flags described in https://www.terraform.io/cli/commands.

In addition, the `component` argument and `stack` flag are required to generate the variables and backend config for the component in the stack.
:::

<br/>

Additions and differences from native `terraform`:

- before executing other `terraform` commands, `atmos` runs `terraform init`
- you can skip over atmos calling `terraform init` if you know your project is already in a good working state by using the `--skip-init` flag like
  so `atmos terraform <command> <component> -s <stack> --skip-init`
- `atmos terraform deploy` command executes `terraform plan` and then `terraform apply`
- `atmos terraform deploy` command supports `--deploy-run-init=true|false` flag to enable/disable running `terraform init` before executing the
  command
- `atmos terraform deploy` command sets `-auto-approve` flag before running `terraform apply`
- `atmos terraform apply` and `atmos terraform deploy` commands support `--from-plan` flag. If the flag is specified, the commands will use the
  previously generated `planfile` instead of generating a new `varfile`
- `atmos terraform clean` command deletes the `.terraform` folder, `.terraform.lock.hcl` lock file, and the previously generated `planfile`
  and `varfile` for the specified component and stack
- `atmos terraform workspace` command first runs `terraform init -reconfigure`, then `terraform workspace select`, and if the workspace was not
  created before, it then runs `terraform workspace new`
- `atmos terraform import` command searches for `region` in the variables for the specified component and stack, and if it finds it,
  sets `AWS_REGION=<region>` ENV var before executing the command
- `atmos terraform generate backend` command generates the backend config file for the component in the stack
- `atmos terraform generate backends` command generates the backend config files for all components in all stacks
- `atmos terraform generate varfile` command generates a varfile for the component in the stack
- `atmos terraform shell` command configures an environment for the component in the stack and starts a new shell allowing executing all native
  terraform commands inside the shell

:::tip
Run `atmos terraform --help` to see all the available options
:::

## Examples

```shell
atmos terraform plan test/test-component-override-3 -s tenant1-ue2-dev
atmos terraform apply test/test-component-override-3 -s tenant1-ue2-dev
atmos terraform destroy test/test-component-override-3 -s tenant1-ue2-dev
atmos terraform init test/test-component-override-3 -s tenant1-ue2-dev
atmos terraform workspace test/test-component-override-3 -s tenant1-ue2-dev
atmos terraform clean test/test-component-override-3 -s tenant1-ue2-dev
```

## Arguments

| Argument     | Description        | Required |
|:-------------|:-------------------|:---------|
| `component`  | `atmos` component  | yes      |

## Flags

| Flag        | Description   | Alias | Required |
|:------------|:--------------|:------|:---------|
| `--stack`   | `atmos` stack | `-s`  | yes      |
| `--dry-run` | Dry-run       |       | no       |

<br/>

:::note
All native `terraform` flags are supported
:::
