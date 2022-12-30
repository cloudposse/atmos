---
title: Create Components
sidebar_position: 5
sidebar_label: Create Components
---

In the previous steps, we've decided on the following:

- Use a monorepo to configure and provision two Terraform components into three AWS accounts and two AWS regions
- The filesystem layout for the infrastructure monorepo
- To be able to use [Component Remote State](/core-concepts/components/remote-state), we put the `atmos.yaml` CLI config file
  into `/usr/local/etc/atmos/atmos.yaml` folder and set the ENV var `ATMOS_BASE_PATH` to point to the absolute path of the root of the repo

Next step is to create Terraform components `vpc-flow-logs-bucket` and `vpc`.

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
   │       │   │   ├── context.tf
   │       │   │   ├── main.tf
   │       │   │   ├── outputs.tf
   │       │   │   ├── providers.tf
   │       │   │   ├── remote-state.tf
   │       │   │   ├── variables.tf
   │       │   │   ├── versions.tf
   │       │   │   ├── vpc-flow-logs.tf
   │       │   ├── vpc-flow-logs-bucket
   │       │   │   ├── context.tf
   │       │   │   ├── main.tf
   │       │   │   ├── outputs.tf
   │       │   │   ├── providers.tf
   │       │   │   ├── variables.tf
   │       │   │   ├── versions.tf
```
