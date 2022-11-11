---
title: "Example #1"
sidebar_position: 3
sidebar_label: Simple Example
---

The [example](https://github.com/cloudposse/atmos/tree/master/examples/complete) folder contains a complete solution that shows how to:

  - Structure the Terraform and helmfile components
  - Configure the CLI
  - Add [stack configurations](examples/complete/stacks) for the Terraform and helmfile components (to provision them to different environments and stages)


## Example Filesystem Layout

This example provides a simple filesystem layout that looks like this:

```console
   │  
   │   # Centralized components configuration
   ├── stacks/
   │   │
   │   └── $stack.yaml
   │  
   │   # Components are broken down by tool
   ├── components/
   │   │
   │   ├── terraform/   # root modules in here
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

## Stack Configuration

`atmos` provides separation of configuration and code, allowing you to provision the same code into different regions, environments and stages.

In our example, all the code (Terraform and helmfiles) is in the [components](https://github.com/cloudposse/atmos/tree/master/examples/complete/components) folder.

The centralized stack configurations (variables for the Terraform and helmfile components) are in the [stacks](https://github.com/cloudposse/atmos/tree/master/examples/complete/stacks) folder.

In the example, all stack configuration files are broken down by environments and stages and use the predefined format `$environment-$stage.yaml`.

Environments are abbreviations of AWS regions, e.g. `ue2` stands for `us-east-2`, whereas `uw2` would stand for `us-west-2`.

`$environment-globals.yaml` is where you define the global values for all stages for a particular environment.
The global values get merged with the `$environment-$stage.yaml` configuration for a specific environment/stage by the CLI.

In the example, we defined a few config files:

  - [ue2-dev.yaml](example/stacks/ue2-dev.yaml) - stack configuration (Terraform and helmfile variables) for the environment `ue2` and stage `dev`
  - [ue2-staging.yaml](example/stacks/ue2-staging.yaml) - stack configuration (Terraform and helmfile variables) for the environment `ue2` and stage `staging`
  - [ue2-prod.yaml](example/stacks/ue2-prod.yaml) - stack configuration (Terraform and helmfile variables) for the environment `ue2` and stage `prod`
  - [ue2-globals.yaml](example/stacks/ue2-globals.yaml) - global settings for the environment `ue2` (e.g. `region`, `environment`)
  - [globals.yaml](example/stacks/globals.yaml) - global settings for the entire solution

__NOTE:__ The stack configuration structure and the file names described above are just an example of how to name and structure the config files.
You can choose any file name for a stack. You can also include other configuration files (e.g. globals for the environment, and globals for the entire solution)
into a stack config using the `import` directive.

Stack configuration files have a predefined format:

```yaml
import:
  - ue2-globals

vars:
  stage: dev

terraform:
  vars: {}

helmfile:
  vars: {}

components:
  terraform:
    vpc:
      command: "/usr/bin/terraform-0.15"
      backend:
        s3:
          workspace_key_prefix: "vpc"
      vars:
        cidr_block: "10.102.0.0/18"
    eks:
      backend:
        s3:
          workspace_key_prefix: "eks"
      vars: {}

  helmfile:
    nginx-ingress:
      vars:
        installed: true
```

It has the following main sections:

- `import` - (optional) a list of stack configuration files to import and combine with the current configuration file
- `vars` - (optional) a map of variables for all components (Terraform and helmfile) in the stack
- `terraform` - (optional) configuration (variables) for all Terraform components
- `helmfile` - (optional) configuration (variables) for all helmfile components
- `components` - (required) configuration for the Terraform and helmfile components

The `components` section consists of the following:

- `terraform` - defines variables, the binary to execute, and the backend for each Terraform component.
  Terraform component names correspond to the Terraform components in the [components](example/components) folder

- `helmfile` - defines variables and the binary to execute for each helmfile component.
  Helmfile component names correspond to the helmfile components in the [helmfile](example/components/helmfile) folder


## Run the Example

To run the example, execute the following commands in a terminal:

- `cd example`
- `make all` - it will build the Docker image, build the CLI tool inside the image, and then start the container
