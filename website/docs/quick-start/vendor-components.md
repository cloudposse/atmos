---
title: Vendor Components
sidebar_position: 5
sidebar_label: Vendor Components
---

In the previous steps, we've configured the repository, and decided to provision the `vpc-flow-logs-bucket` and `vpc` Terraform
components into three AWS accounts (`dev`, `staging`, `prod`) in two AWS regions (`us-east-2` and `us-west-2`). We've also configured the Atmos CLI to
search for the Terraform components in the `components/terraform` directory.

Next step is to create the Terraform components `vpc-flow-logs-bucket` and `vpc`.

There are a few ways to create the Terraform components:

- Copy the `vpc-flow-logs-bucket` component from the Atmos
  example [components/terraform/vpc-flow-logs-bucket](https://github.com/cloudposse/atmos/tree/master/examples/quick-start/components/terraform/vpc-flow-logs-bucket)

- Copy the `vpc` component from the Atmos
  example [components/terraform/vpc](https://github.com/cloudposse/atmos/tree/master/examples/quick-start/components/terraform/vpc)

or

- Copy the `component.yaml` component vendoring config file from the Atmos
  example [components/terraform/vpc-flow-logs-bucket/component.yaml](https://github.com/cloudposse/atmos/blob/master/examples/quick-start/components/terraform/vpc-flow-logs-bucket/component.yaml)
  into `components/terraform/vpc-flow-logs-bucket/component.yaml` and then run the Atmos
  command `atmos vendor pull --component vpc-flow-logs-bucket` from
  the root of the repo. The command will copy all the component's files from the open-source component
  repository [terraform-aws-components](https://github.com/cloudposse/terraform-aws-components/tree/master/modules/vpc-flow-logs-bucket)

- Copy the `component.yaml` component vendoring config file from the Atmos
  example [components/terraform/vpc/component.yaml](https://github.com/cloudposse/atmos/blob/master/examples/quick-start/components/terraform/vpc/component.yaml)
  into `components/terraform/vpc/component.yaml` and then run the Atmos command `atmos vendor pull --component vpc` from
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
   │       ├── vpc
   │       │   ├── context.tf
   │       │   ├── main.tf
   │       │   ├── outputs.tf
   │       │   ├── providers.tf
   │       │   ├── remote-state.tf
   │       │   ├── variables.tf
   │       │   ├── versions.tf
   │       │   ├── vpc-flow-logs.tf
   │       ├── vpc-flow-logs-bucket
   │       │   ├── context.tf
   │       │   ├── main.tf
   │       │   ├── outputs.tf
   │       │   ├── providers.tf
   │       │   ├── variables.tf
   │       │   ├── versions.tf
```

<br/>

Each component follows the [Standard Module Structure](https://developer.hashicorp.com/terraform/language/modules/develop/structure) that Terraform
recommends. There are a few additions:

- `context.tf` - this file contains all the common variables that Terraform modules and components consume (to make the component's `variables.tf`
  file DRY). This is a standard file that is copied into each component. The file also defines the context
  variables (`namespace`, `tenant`, `environment`, `stage`) which are used by Atmos to search for Atmos stacks when executing
  the [CLI commands](/cli/cheatsheet)

- `remote-state.tf` in the `vpc` component - this file configures the
  [remote-state](https://github.com/cloudposse/terraform-yaml-stack-config/tree/main/modules/remote-state) Terraform module to obtain the remote state
  for the `vpc-flow-logs-bucket` component. The `vpc` Terraform component needs the outputs from the `vpc-flow-logs-bucket` Terraform component to
  configure [VPC Flow Logs](https://docs.aws.amazon.com/vpc/latest/userguide/flow-logs.html) and store them in the S3 bucket

```hcl title="components/terraform/vpc/remote-state.tf"
module "vpc_flow_logs_bucket" {
  count = var.vpc_flow_logs_enabled ? 1 : 0

  source  = "cloudposse/stack-config/yaml//modules/remote-state"
  version = "1.5.0"

  # Specify the Atmos component name (defined in YAML stack config files) 
  # for which to get the remote state outputs
  component = "vpc-flow-logs-bucket"

  # Override the context variables to point to a different Atmos stack if the 
  # `vpc-flow-logs-bucket` Atmos component is provisioned in another AWS account, OU or region
  environment = var.vpc_flow_logs_bucket_environment_name
  stage       = var.vpc_flow_logs_bucket_stage_name
  tenant      = try(coalesce(var.vpc_flow_logs_bucket_tenant_name, module.this.tenant), null)

  # `context` input is a way to provide the information about the stack (using the context
  # variables `namespace`, `tenant`, `environment`, `stage` defined in the stack config)
  context = module.this.context
}
```

<br/>

For a complete description of how Atmos components use remote state, refer to [Component Remote State](/core-concepts/components/remote-state).
