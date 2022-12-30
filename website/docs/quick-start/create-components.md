---
title: Create Components
sidebar_position: 5
sidebar_label: Create Components
---

In the previous steps, we've configured the repository, and decided to provision the `vpc-flow-logs-bucket` and `vpc` Terraform
components into three AWS accounts (`dev`, `staging`, `prod`) in two AWS regions (`us-east-2` and `us-west-2`). We've also configured the Atmos CLI to
search for the components in the `components/terraform` directory.

We'll also put the Terraform components into the `infra` folder under `components/terraform` (note that a component can be in
the `components/terraform` directory itself, or in any subfolder at any level in the directory).

Next step is to create the Terraform components `vpc-flow-logs-bucket` and `vpc`.

There are a few ways to create the Terraform components:

- Copy the `vpc-flow-logs-bucket` component from the Atmos
  example [components/terraform/infra/vpc-flow-logs-bucket](https://github.com/cloudposse/atmos/tree/master/examples/complete/components/terraform/infra/vpc-flow-logs-bucket)

- Copy the `vpc` component from the Atmos
  example [components/terraform/infra/vpc](https://github.com/cloudposse/atmos/tree/master/examples/complete/components/terraform/infra/vpc)

or

- Copy the `component.yaml` component vendoring config file from the Atmos
  example [components/terraform/infra/vpc-flow-logs-bucket/component.yaml](https://github.com/cloudposse/atmos/blob/master/examples/complete/components/terraform/infra/vpc-flow-logs-bucket/component.yaml)
  into `components/terraform/infra/vpc-flow-logs-bucket/component.yaml` and then run the Atmos
  command `atmos vendor pull --component infra/vpc-flow-logs-bucket` from
  the root of the repo. The command will copy all the component's files from the open-source component
  repository [terraform-aws-components](https://github.com/cloudposse/terraform-aws-components/tree/master/modules/vpc-flow-logs-bucket)

- Copy the `component.yaml` component vendoring config file from the Atmos
  example [components/terraform/infra/vpc/component.yaml](https://github.com/cloudposse/atmos/blob/master/examples/complete/components/terraform/infra/vpc/component.yaml)
  into `components/terraform/infra/vpc/component.yaml` and then run the Atmos command `atmos vendor pull --component infra/vpc` from
  the root of the repo. The command will copy all the component's files from the open-source component
  repository [terraform-aws-components](https://github.com/cloudposse/terraform-aws-components/tree/master/modules/vpc)

The filesystem layout should look like this:

<br/>

```console
   │  
   │   # Centralized stacks configuration
   ├── stacks
   │   └── <stack_1>
   │   └── <stack_2>
   │   └── <stack_3>
   │  
   │   # Centralized components configuration. Components are broken down by tool
   ├── components
   │   └── terraform   # Terraform components (Terraform root modules)
   |       ├── infra
   │       │   ├── vpc
   │       │   │   ├── component.yaml
   │       │   │   ├── context.tf
   │       │   │   ├── main.tf
   │       │   │   ├── outputs.tf
   │       │   │   ├── providers.tf
   │       │   │   ├── remote-state.tf
   │       │   │   ├── variables.tf
   │       │   │   ├── versions.tf
   │       │   │   ├── vpc-flow-logs.tf
   │       │   ├── vpc-flow-logs-bucket
   │       │   │   ├── component.yaml
   │       │   │   ├── context.tf
   │       │   │   ├── main.tf
   │       │   │   ├── outputs.tf
   │       │   │   ├── providers.tf
   │       │   │   ├── variables.tf
   │       │   │   ├── versions.tf
```

<br/>

Each component follows the [Standard Module Structure](https://developer.hashicorp.com/terraform/language/modules/develop/structure) that Terraform
recommends. There are a few additions:

- `context.tf` - this file contains all the common variables that all Terraform modules and components consume (to make the component's `variables.tf`
  file DRY). This is a standard file that is copied into each component. The file also defines the context
  variables (`namespace`, `tenant`, `environment`, `stage`) which are used by Atmos to search for Atmos stacks when executing
  the [CLI commands](/cli/cheatsheet)

- `remote-state.tf` in the `vpc` component

