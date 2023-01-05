---
title: atmos terraform deploy
sidebar_label: deploy
sidebar_class_name: command
id: deploy
---

:::note purpose
Use this command to execute `terraform plan` and then `terraform apply` on an Atmos component in an Atmos stack.
:::

## Usage

Execute the `terraform deploy` subcommand like this:

```shell
atmos terraform deploy <component> -s <stack>
```

- `atmos terraform deploy` command supports `--deploy-run-init=true|false` flag to enable/disable running `terraform init` before executing the
  command

- `atmos terraform deploy` command automatically sets `-auto-approve` flag before running `terraform apply`

- `atmos terraform deploy` command supports `--from-plan` flag. If the flag is specified, the commands will use the
  previously generated `planfile` instead of generating a new `varfile`
See [all flags](#Flags).
<br/>

:::tip
Run `atmos terraform deploy --help` to see all the available options
:::

## Examples

```shell
atmos terraform deploy top-level-component1 -s tenant1-ue2-dev
atmos terraform deploy infra/vpc -s tenant1-ue2-staging
atmos terraform deploy test/test-component -s tenant1-ue2-dev
atmos terraform deploy test/test-component-override-2 -s tenant2-ue2-prod
atmos terraform deploy test/test-component-override-3 -s tenant1-ue2-dev
```

## Arguments

| Argument    | Description               | Required |
|:------------|:--------------------------|:---------|
| `component` | Atmos terraform component | yes      |

## Flags

| Flag                | Description                                                                                             | Alias | Required |
|:--------------------|:--------------------------------------------------------------------------------------------------------|:------|:---------|
| `--stack`           | Atmos stack                                                                                             | `-s`  | yes      |
| `--dry-run`         | Dry run                                                                                                 |       | no       |
| `--deploy-run-init` | Enable/disable running `terraform init` before executing the command                                    |       | no       |
| `--from-plan`       | If the flag is specified, use the previously generated `planfile` instead of generating a new `varfile` |       | no       |

<br/>

:::note

The command supports all native Terraform options described
in [apply options](https://developer.hashicorp.com/terraform/cli/commands/apply#apply-options)

:::
