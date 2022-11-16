---
title: atmos vendor pull
sidebar_label: pull
sidebar_class_name: command
id: pull
description: Use this command to pull sources and mixins from remote repositories for a `terraform` or `helmfile` component.
---

:::note Purpose
Use this command to pull sources and mixins from remote repositories for a `terraform` or `helmfile` component.
:::

## Usage

Execute the `vendor pull` command like this:

```shell
atmos vendor pull --component <component> [options]
atmos vendor pull -c <component> [options]
```

This command pulls sources and mixins from remote repositories for a `terraform` or `helmfile` component.

- Supports Kubernetes-style YAML config (file `component.yaml`) to describe component vendoring configuration. The file is placed into the component's
  folder

- The URIs (`uri`) in `component.yaml` support all protocols (local files, Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP), and all URL and
  archive formats as described in https://github.com/hashicorp/go-getter

- `included_paths` and `excluded_paths` in `component.yaml` support [POSIX-style greedy Globs](https://en.wikipedia.org/wiki/Glob_(programming)) for
  file names/paths (double-star/globstar `**` is supported as well)

<br/>

:::tip
Run `atmos vendor pull --help` to see all the available options
:::

## Examples

```shell
atmos vendor pull -c infra/account-map
atmos vendor pull -c infra/vpc-flow-logs-bucket
atmos vendor pull -c echo-server -t helmfile
atmos vendor pull -c infra/account-map --dry-run
```

## Flags

| Flag          | Description                                                        | Alias | Required |
|:--------------|:-------------------------------------------------------------------|:------|:---------|
| `--component` | `atmos` component to pull sources and mixins for                   | `-c`  | yes      |
| `--type`      | Component type: `terraform` or `helmfile` (`terraform` is default) | `-t`  | no       |
| `--dry-run`   | Dry-run                                                            |       | no       |
