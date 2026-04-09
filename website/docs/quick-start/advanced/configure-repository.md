---
title: Configure Project Repository
sidebar_position: 3
sidebar_label: Configure Project Repository
---

Atmos supports both the [monorepo and polyrepo architectures](https://en.wikipedia.org/wiki/Monorepo) when managing the configurations for components
and stacks.

:::info

A "monorepo" is a version-controlled repository that stores all the code, configurations and scripts for the entire infrastructure composed of individual components with independent lifecycles. Monorepos usually improve collaboration, CI/CD build speed, and overall productivity. A monorepo should not be confused with a [monolith](https://en.wikipedia.org/wiki/Monolithic_application), which is a single, often large, codebase for an application.

Polyrepo architectures consists of several version-controlled repositories for code, configurations and scripts for different parts of the
infrastructure. For example, depending on various requirements (including security, lifecycle management, access control, audit, etc.), separate repositories can be used to manage infrastructure per account (e.g. `dev`, `staging`, `prod`), per service, or per team.

:::

<br/>

In this Quick Start guide, we will be using a monorepo to provision the following resources into multiple AWS accounts (`dev`, `staging`, `prod`)
and regions (`us-east-2` and `us-west-2`):

- [vpc-flow-logs-bucket](https://github.com/cloudposse/atmos/tree/main/examples/quick-start-advanced/components/terraform/vpc-flow-logs-bucket)
- [vpc](https://github.com/cloudposse/atmos/tree/main/examples/quick-start-advanced/components/terraform/vpc)

## Common Directories and Files

Atmos requires a few common directories and files, which need to be configured in the infrastructure repo:

- `components` directory (required) - contains centralized component configurations
- `stacks` directory (required) - contains centralized stack configurations
- `atmos.yaml` (required) - CLI config file
- `vendor.yaml` (optional) - Atmos vendor config file
- `Makefile` (optional)
- `Dockerfile` (optional)
- `rootfs` directory (optional) - root filesystem for the Docker image (if `Dockerfile` is used)

Atmos separates code from configuration (separation of concerns). The code is in the `components` directories and the configurations for different environments are in the `stacks` directory. This allows the code (Terraform and Helmfile components) to be environment-agnostic, meaning the components don't know and don't care how and where they will be provisioned. They can be provisioned into many accounts and regions - the configurations for different environments are defined in the `stacks` directory.

<br/>

:::note
While it's recommended to use the directory names as shown above, the `stacks` and `components` directory names and filesystem locations are
configurable in the `atmos.yaml` CLI config file. Refer to [Configure CLI](/quick-start/advanced/configure-cli) for more details.
:::

<br/>

The following example provides the simplest filesystem layout that Atmos can work with:

```console
   │   # Centralized stacks configuration
   ├── stacks/
   │   ├── <stack_1>.yaml
   │   ├── <stack_2>.yaml
   │   └── <stack_3>.yaml
   │  
   │   # Centralized components configuration. Components are broken down by tool
   ├── components/
   │   ├── terraform/   # Terraform components (Terraform root modules)
   │   │   ├── <terraform_component_1>/
   │   │   ├── <terraform_component_2>/
   │   │   └── <terraform_component_3>/
   │   └── helmfile/  # Helmfile components are organized by Helm chart
   │       ├── <helmfile_component_1>/
   │       ├── <helmfile_component_2>/
   │       └── <helmfile_component_3>/
   │
   │   # Atmos CLI configuration
   ├── atmos.yaml
   │   # Atmos vendoring configuration
   └── vendor.yaml
```

<br/>

## `atmos.yaml` CLI Config File Location

While placing `atmos.yaml` at the root of the repository will work for the `atmos` CLI, it will not work
for [Component Remote State](/stacks/remote-state) because it uses
the [terraform-provider-utils](https://github.com/cloudposse/terraform-provider-utils) Terraform provider. Terraform executes the provider from the
component's folder (e.g. `components/terraform/vpc`), and we don't want to replicate `atmos.yaml` into every component's folder.

Both the `atmos` CLI and [terraform-provider-utils](https://github.com/cloudposse/terraform-provider-utils) Terraform provider use the same `Go` code,
which try to locate the [CLI config](/cli/configuration) `atmos.yaml` file before parsing and processing [Atmos stacks](/learn/stacks).

This means that `atmos.yaml` file must be at a location in the file system where all processes can find it.

:::info

`atmos.yaml` is loaded from the following locations (from lowest to highest priority):

- System dir (`/usr/local/etc/atmos/atmos.yaml` on Linux, `%LOCALAPPDATA%/atmos/atmos.yaml` on Windows)
- Home dir (`~/.atmos/atmos.yaml`)
- Current directory
- ENV var `ATMOS_CLI_CONFIG_PATH`

:::

:::note

Initial Atmos configuration can be controlled by these ENV vars:

- `ATMOS_CLI_CONFIG_PATH` - where to find `atmos.yaml`. Path to a folder where the `atmos.yaml` CLI config file is located (just the folder without
  the file name)

- `ATMOS_BASE_PATH` - base path to `components` and `stacks` folders

:::

<br/>

For this to work for both the `atmos` CLI and the Terraform provider, we recommend doing one of the following:

- Put `atmos.yaml` at `/usr/local/etc/atmos/atmos.yaml` on local host and set the ENV var `ATMOS_BASE_PATH` to point to the absolute path of the root
  of the repo

- Put `atmos.yaml` into the home directory (`~/.atmos/atmos.yaml`) and set the ENV var `ATMOS_BASE_PATH` pointing to the absolute path of the root of
  the repo

- Put `atmos.yaml` at a location in the file system and then set the ENV var `ATMOS_CLI_CONFIG_PATH` to point to that location. The ENV var must
  point to a folder without the `atmos.yaml` file name. For example, if `atmos.yaml` is at `/atmos/config/atmos.yaml`,
  set `ATMOS_CLI_CONFIG_PATH=/atmos/config`. Then set the ENV var `ATMOS_BASE_PATH` pointing to the absolute path of the root of the repo

- When working in a Docker container, place `atmos.yaml` in the `rootfs` directory
  at [/rootfs/usr/local/etc/atmos/atmos.yaml](https://github.com/cloudposse/atmos/blob/main/examples/quick-start-advanced/rootfs/usr/local/etc/atmos/atmos.yaml)
  and then copy it into the container's file system in the [Dockerfile](https://github.com/cloudposse/atmos/blob/main/examples/quick-start-advanced/Dockerfile)
  by executing the `COPY rootfs/ /` Docker command. Then in the Dockerfile, set the ENV var `ATMOS_BASE_PATH` pointing to the absolute path of the
  root of the repo. Note that the [Atmos Quick Start example](https://github.com/cloudposse/atmos/blob/main/examples/quick-start)
  uses [Geodesic](https://github.com/cloudposse/geodesic) as the base Docker image. [Geodesic](https://github.com/cloudposse/geodesic) sets the ENV
  var `ATMOS_BASE_PATH` automatically to the absolute path of the root of the repo on the local host

## Final Filesystem Layout

Taking into account all the above, we can place `atmos.yaml` at `/usr/local/etc/atmos/atmos.yaml` on the local host and use the following filesystem
layout:

```console
   │   # Centralized stacks configuration
   ├── stacks/
   │   ├── <stack_1>.yaml
   │   ├── <stack_2>.yaml
   │   └── <stack_3>.yaml
   │  
   │   # Centralized components configuration. Components are broken down by tool
   └── components/
       └── terraform/   # Terraform components (Terraform root modules)
           ├── vpc/
           └── vpc-flow-logs-bucket/
```

<br/>

:::tip

For a Quick Start example, refer to [Atmos Quick Start](https://github.com/cloudposse/atmos/tree/main/examples/quick-start-advanced)

:::
