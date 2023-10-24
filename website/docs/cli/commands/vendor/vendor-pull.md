---
title: atmos vendor pull
sidebar_label: pull
sidebar_class_name: command
id: pull
description: Use this command to pull sources and mixins from remote repositories for Terraform and Helmfile components and stacks.
---

:::note Purpose
This command implements [Atmos Vendoring](/core-concepts/vendoring/). Use this command to download all sources and mixins from remote repositories for Terraform and Helmfile components and stacks.
:::

## Usage

Execute the `vendor pull` command like this:

```shell
atmos vendor pull
atmos vendor pull --component <component> [options]
atmos vendor pull -c <component> [options]
```

## Description

Atmos supports two different ways of vendoring components, stacks and other artifacts:

- Using `vendor.yaml` vendoring manifest
- Using `component.yaml` vendoring manifest

The `component.yaml` vendoring manifest can be used to vendor components from remote repositories.
A `component.yaml` file placed into a component's directory is used to describe the vendoring config for one component only.
Using `component.yaml` is not recommended, and it's maintained for backwards compatibility.

The `vendor.yaml` vendoring manifest provides more functionality than using `component.yaml` files.
It's used to describe vendoring config for all components, stacks and other artifacts for the entire infrastructure.
The file is placed into the directory from which the `atmos vendor pull` command is executed. It's the recommended way to describe vendoring
configurations.

## Vendoring using `vendor.yaml` manifest

- The `vendor.yaml` vendoring manifest supports Kubernetes-style YAML config to describe vendoring configuration for components, stacks,
  and other artifacts. The file is placed into the directory from which the `atmos vendor pull` command is executed (usually the root of the repo)

- The `sources` in `vendor.yaml` support all protocols (local files, Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP), and all URL and
  archive formats as described in [go-getter](https://github.com/hashicorp/go-getter), and also the `oci://` scheme to download artifacts from
  [OCI registries](https://opencontainers.org).

- The `targets` in the sources support absolute and relative paths (relative to the directory where the command is executed)

- `included_paths` and `excluded_paths` support [POSIX-style greedy Globs](https://en.wikipedia.org/wiki/Glob_(programming)) for filenames/paths
  (double-star/globstar `**` is supported as well)

:::tip
Refer to [`Atmos Vendoring`](/core-concepts/vendoring) for more details
:::

## Vendoring using `component.yaml` manifest

- The `component.yaml` vendoring manifest supports Kubernetes-style YAML config to describe component vendoring configuration.
  The file is placed into the component's folder

- The URIs (`uri`) in `component.yaml` support all protocols (local files, Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP), and all URL and
  archive formats as described in [go-getter](https://github.com/hashicorp/go-getter), and also the `oci://` scheme to download artifacts from
  [OCI registries](https://opencontainers.org).

- `included_paths` and `excluded_paths` in `component.yaml` support [POSIX-style greedy Globs](https://en.wikipedia.org/wiki/Glob_(programming)) for
  file names/paths (double-star/globstar `**` is supported as well)

:::tip
Refer to [`Atmos Component Vendoring`](/core-concepts/components/vendoring) for more details
:::

## Vendoring from OCI Registries

The following config can be used to download the `vpc` component from an AWS public ECR registry:

  ```yaml
  apiVersion: atmos/v1
  kind: ComponentVendorConfig
  metadata:
    name: vpc-vendor-config
    description: Config for vendoring of 'vpc' component
  spec:
    source:
      # Download the component from the AWS public ECR registry (https://docs.aws.amazon.com/AmazonECR/latest/public/public-registries.html)
      uri: "oci://public.ecr.aws/cloudposse/components/terraform/stable/aws/vpc:{{.Version}}"
      version: "latest"
  ```

<br/>

:::tip
Run `atmos vendor pull --help` to see all the available options
:::

## Examples

```shell
atmos vendor pull
atmos vendor pull -c infra/account-map
atmos vendor pull -c infra/vpc-flow-logs-bucket
atmos vendor pull -c echo-server -t helmfile
atmos vendor pull -c infra/account-map --dry-run
```

<br/>

:::note

When executing the `atmos vendor pull` command, Atmos performs the following steps to decide which vendoring manifest to use:

- If `vendor.yaml` manifest is found (in the directory from which the command is executed), Atmos will parse the file and execute the command
  against it. If the flag `--component` is not specified, Atmos will vendor all the artifacts defined in the `vendor.yaml` manifest.
  If the flag `--component` is passed in, Atmos will vendor only that component

- If `vendor.yaml` is not found, Atmos will look for the `component.yaml` manifest in the component's folder. If `component.yaml` is not found,
  an error will be thrown. The flag `--component` is required in this case

:::

## Flags

| Flag          | Description                                                        | Alias | Required |
|:--------------|:-------------------------------------------------------------------|:------|:---------|
| `--component` | Atmos component to pull sources and mixins for                     | `-c`  | no       |
| `--type`      | Component type: `terraform` or `helmfile` (`terraform` is default) | `-t`  | no       |
| `--dry-run`   | Dry run                                                            |       | no       |
