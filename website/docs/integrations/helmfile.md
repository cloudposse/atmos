---
title: Helmfile Integration
sidebar_position: 4
sidebar_label: Helmfile
---

Atmos natively supports opinionated workflows for [Helmfile](https://github.com/helmfile/helmfile).

Helmfile provides a declarative specification for deploying helm charts.

For a complete list of supported commands, please see the Atmos [helmfile](/cli/commands/helmfile/usage) documentation.

## Example: Provision Helmfile Component

To provision a helmfile component using the `atmos` CLI, run the following commands in the container shell:

```shell
atmos helmfile diff nginx-ingress --stack=ue2-dev
atmos helmfile apply nginx-ingress --stack=ue2-dev
```

where:

- `nginx-ingress` is the helmfile component to provision (from the `components/helmfile` folder)
- `--stack=ue2-dev` is the stack to provision the component into

Short versions of the command-line arguments can be used:

```shell
atmos helmfile diff nginx-ingress -s ue2-dev
atmos helmfile apply nginx-ingress -s ue2-dev
```

## Example: Helmfile Diff

To execute `diff` and `apply` in one step, use `helmfile deploy` command:

```shell
atmos helmfile deploy nginx-ingress -s ue2-dev
```

