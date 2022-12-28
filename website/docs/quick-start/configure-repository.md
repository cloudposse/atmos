---
title: Configure Repository
sidebar_position: 3
sidebar_label: Configure Repository
---

Atmos supports both the [monorepo and polyrepo architectures](https://en.wikipedia.org/wiki/Monorepo) when managing the configurations for components
and stacks.

:::info

Monorepo is a version-controlled repository that stores all the code, configurations and scripts for the entire infrastructure.
Monorepo usually improves collaboration, CI build speed, and overall productivity.

Polyrepo architecture consists of several version-controlled repositories for code, configurations and scripts for different parts of the
infrastructure. For example, depending on various requirements including security, lifecycle management, access control, audit, etc., separate
repositories can be used to manage infrastructure per account (e.g. `dev`, `staging`, `prod`), per service, or per team.

:::

<br/>

In this Quick Start guide, we will be using a monorepo to provision the following resources into multiple AWS accounts and regions:

- [vpc-flow-logs-bucket](https://github.com/cloudposse/atmos/tree/master/examples/complete/components/terraform/infra/vpc-flow-logs-bucket)
- [vpc](https://github.com/cloudposse/atmos/tree/master/examples/complete/components/terraform/infra/vpc)

Atmos requires a few common directories and files, which need to be configured in the infrastructure repo:

- `stacks` directory (required)
- `components` directory (required)
- `atmos.yaml` CLI config file (required)
- `Makefile` (optional)
- `Dockerfile` (optional)
- `rootfs` directory (optional)

<br/>

:::note
While it's recommended to use the directory names as shown above, the `stacks` and `components` directory names and filesystem locations are
configurable in the `atmos.yaml` CLI config file. Refer to [Configure CLI](/quick-start/configure-cli) for more details.
:::

<br/>

The following example provides the simplest filesystem layout that Atmos can work with:

```console
   │  
   │   # Centralized stacks configuration
   ├── stacks/
   │   │
   │   └── <stack_1>.yaml
   │   └── <stack_2>.yaml
   │   └── <stack_3>.yaml
   │  
   │   # Centralized components configuration. Components are broken down by tool
   ├── components/
   │   │
   │   ├── terraform/   # Terraform components (Terraform root modules)
   │   │   ├── <terraform_component_1>/
   │   │   ├── <terraform_component_2>/
   │   │   ├── <terraform_component_3>/
   │   │
   │   └── helmfile/  # Helmfile components are organized by Helm chart
   │   │   ├── <helmfile_component_1>/
   │   │   ├── <helmfile_component_2>/
   │   │   ├── <helmfile_component_3>/
   │
   │   # Atmos CLI configuration
   ├── atmos.yaml
```

```console
   │  
   │   # Centralized stacks configuration
   ├── stacks/
   │   │
   │   └── $stack.yaml
   │  
   │   # Centralized components configuration. Components are broken down by tool
   ├── components/
   │   │
   │   ├── terraform/   # Terraform root modules
   │   │   ├── infra/
   │   │   ├── mixins/
   │   │   ├── test/test-component/
   │   │   └── top-level-component1/
   │   │
   │   └── helmfile/  # helmfiles are organized by chart
   │       ├── echo-server/
   │       └── infra/infra-server
   │  
   │   # Root filesystem for the docker image (see `Dockerfile`)
   ├── rootfs/
   │
   │   # Makefile for building the CLI
   ├── Makefile
   │   # Atmos CLI configuration
   ├── atmos.yaml
   │  
   │   # Docker image for shipping the CLI and all dependencies
   └── Dockerfile (optional)
```

```console
   │  
   │   # Centralized stacks configuration
   ├── stacks/
   │   │
   │   └── $stack.yaml
   │  
   │   # Centralized components configuration. Components are broken down by tool
   ├── components/
   │   │
   │   ├── terraform/   # Terraform root modules
   │   │   ├── infra/
   │   │   ├── mixins/
   │   │   ├── test/test-component/
   │   │   └── top-level-component1/
   │   │
   │   └── helmfile/  # helmfiles are organized by chart
   │       ├── echo-server/
   │       └── infra/infra-server
   │  
   │   # Root filesystem for the docker image (see `Dockerfile`)
   ├── rootfs/
   │
   │   # Makefile for building the CLI
   ├── Makefile
   │   # Atmos CLI configuration
   ├── atmos.yaml
   │  
   │   # Docker image for shipping the CLI and all dependencies
   └── Dockerfile (optional)
```
