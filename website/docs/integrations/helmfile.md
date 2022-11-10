---
sidebar_position: 9
title: Helmfile Integration
---
---

# Helmfile Integration

Atmos natively supports opinionated workflows for Helmfile.


 ## Example: Provision Helmfile Component

To provision a helmfile component using the `atmos` CLI, run the following commands in the container shell:

```console
atmos helmfile diff nginx-ingress --stack=ue2-dev
atmos helmfile apply nginx-ingress --stack=ue2-dev
```

where:

- `nginx-ingress` is the helmfile component to provision (from the `components/helmfile` folder)
- `--stack=ue2-dev` is the stack to provision the component into

Short versions of the command-line arguments can be used:

```console
atmos helmfile diff nginx-ingress -s ue2-dev
atmos helmfile apply nginx-ingress -s ue2-dev
```

## Example: Helmfile Diff

To execute `diff` and `apply` in one step, use `helmfile deploy` command:

```console
atmos helmfile deploy nginx-ingress -s ue2-dev
```

