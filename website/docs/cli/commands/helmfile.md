---
title: atmos helmfile
sidebar_label: helmfile
---

Executes `helmfile` commands.

```shell
atmos helmfile <command> <component> -s <stack> [options]
atmos helmfile <command> <component> --stack <stack> [options]
```

<br/>

:::info
`atmos` supports all `helmfile` commands and options described in [Helmfile CLI reference](https://github.com/helmfile/helmfile#cli-reference).

In addition, the `component` argument and `stack` flag are required to generate variables for the component in the stack.
:::

<br/>

**Additions and differences from native `helmfile`:**

- `atmos helmfile generate varfile` command generates a varfile for the component in the stack
- `atmos helmfile` commands support [GLOBAL OPTIONS](https://github.com/roboll/helmfile#cli-reference) using the command-line flag `--global-options`.
  Usage: `atmos helmfile <command> <component> -s <stack> [command options] [arguments] --global-options="--no-color --namespace=test"`
- before executing the `helmfile` commands, `atmos` runs `aws eks update-kubeconfig` to read kubeconfig from the EKS cluster and use it to
  authenticate with the cluster. This can be disabled in `atmos.yaml` CLI config by setting `components.helmfile.use_eks` to `false`

:::tip
Run `atmos helmfile --help` to see all the available options
:::

## Examples

```shell
atmos helmfile diff echo-server -s tenant1-ue2-dev
atmos helmfile apply echo-server -s tenant1-ue2-dev
atmos helmfile sync echo-server --stack tenant1-ue2-dev
atmos helmfile destroy echo-server --stack=tenant1-ue2-dev
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
All native `helmfile` flags, command options, and arguments are supported
:::
