---
title: Configure Repository
sidebar_position: 3
sidebar_label: Configure Repository
---

Atmos supports both the [monorepo and polyrepo architectures](https://en.wikipedia.org/wiki/Monorepo) when managing the configurations for components
and stacks.

:::info

Monorepo is a version-controlled code repository that stores all the code, configurations, scripts and libraries for the entire infrastructure.
Monorepo usually improves collaboration, CI build speed, and overall productivity.

Polyrepo architecture consists of several version-controlled repositories for code, configurations, scripts and libraries for different parts of the
infrastructure.

:::

<br/>

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
