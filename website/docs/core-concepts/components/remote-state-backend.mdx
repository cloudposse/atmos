---
title: Remote State Backend
sidebar_position: 16
sidebar_label: Remote State Backend
id: remote-state-backend
---

Atmos supports configuring [Terraform Backends](/core-concepts/components/terraform-backends) to define where
Terraform stores its [state](https://developer.hashicorp.com/terraform/language/state),
and [Remote State](/core-concepts/components/remote-state) to get the outputs
of a [Terraform component](/core-concepts/components), provisioned in the same or a
different [Atmos stack](/core-concepts/stacks), and use
the outputs as inputs to another Atmos component

Atmos also supports Remote State Backends (in the `remote_state_backend` section), which can be used to configure the
following:

- Override [Terraform Backend](/core-concepts/components/terraform-backends) configuration to access the
  remote state of a component (e.g. override the IAM role to assume, which in this case can be a read-only role)

- Configure a remote state of type `static` which can be used to provide configurations for
  [Brownfield development](https://en.wikipedia.org/wiki/Brownfield_(software_development))

## Override Terraform Backend Configuration to Access Remote State

Atmos supports the `remote_state_backend` section which can be used to provide configuration to access the remote state
of components.

To access the remote state of components, you can override
any [Terraform Backend](/core-concepts/components/terraform-backends)
configuration in the `backend` section using the `remote_state_backend` section. The `remote_state_backend` section
is a first-class section, and it can be defined globally at any scope (organization, tenant, account, region), or per
component, and then deep-merged using [Atmos Component Inheritance](/core-concepts/components/inheritance).

For example, let's suppose we have the following S3 backend configuration for the entire organization
(refer to [AWS S3 Backend](/core-concepts/components/terraform-backends#aws-s3-backend) for more details):

```yaml title="stacks/orgs/acme/_defaults.yaml"
terraform:
  backend_type: s3
  backend:
    s3:
      acl: "bucket-owner-full-control"
      encrypt: true
      bucket: "your-s3-bucket-name"
      dynamodb_table: "your-dynamodb-table-name"
      key: "terraform.tfstate"
      region: "your-aws-region"
      role_arn: "arn:aws:iam::xxxxxxxx:role/terraform-backend-read-write"
```

<br/>

Let's say we also have a read-only IAM role, and we want to use it to access the remote state instead of the read-write
role, because accessing remote state is a read-only operation, and we don't want to give the role more permissions than
it requires - this is the [principle of least privilege](https://en.wikipedia.org/wiki/Principle_of_least_privilege).

We can add the `remote_state_backend` and `remote_state_backend_type` to override the required attributes from the
`backend` section:

```yaml title="stacks/orgs/acme/_defaults.yaml"
terraform:
  backend_type: s3  # s3, remote, vault, azurerm, gcs, cloud
  backend:
    s3:
      acl: "bucket-owner-full-control"
      encrypt: true
      bucket: "your-s3-bucket-name"
      dynamodb_table: "your-dynamodb-table-name"
      key: "terraform.tfstate"
      region: "your-aws-region"
      role_arn: "arn:aws:iam::xxxxxxxx:role/terraform-backend-read-write"

  remote_state_backend_type: s3 # s3, remote, vault, azurerm, gcs, cloud, static
  remote_state_backend:
    s3:
      role_arn: "arn:aws:iam::xxxxxxxx:role/terraform-backend-read-only"
      # Override the other attributes from the `backend.s3` section as needed
```

<br/>

In the example above, we've overridden the `role_arn` attribute for the `s3` backend to use the read-only role when
accessing the remote state of all components. All other attributes will be taken from the `backend` section (Atmos
deep-merges the `remote_state_backend` section with the `backend` section).

When working with Terraform backends and writing/updating the state, the `terraform-backend-read-write` role will be
used. But when reading the remote state of components, the `terraform-backend-read-only` role will be used.

## Brownfield Considerations

The term "brownfield" comes from urban planning and refers to the redevelopment of land that was previously used and may need cleaning or modification. As it relates to infrastructure, [Brownfield development](https://en.wikipedia.org/wiki/Brownfield_(software_development)) describes development and deployment of new software
systems in the presence of existing (legacy) software applications/systems. Anytime this happens, new software architectures must take into account and coexist with the existing software. 

In the context of Atmos, brownfield development involves adopting some new techniques, practices, and integrating them into
established systems. This can be challenging due to several factors:

- **Legacy Systems**: These are older systems that are still in use and may be harder to change. They might not even be managed with Terraform, or if they are, they might not provide outputs that are easily accessible in a conventional manner.

- **Complex Integrations**: Existing systems might leverage entirely different toolchains, such as CloudFormation, CDK, or Pulumi. Managing integrations and dependencies across toolchains is complicated.


- **Technical Debt**: Over time, systems naturally accumulate technical debt, which includes outdated code, inconsistent conventions or lack of naming conventions, and suboptimal decisions that were previously made for expediency. Addressing this technical debt is crucial when adopting Atmos, as it introduces practices that ensure your system remains maintainable and scalable.


## Brownfield Development in Atmos

Atmos is easier for new organizations or "greenfield" environments because you need to architect Terraform according to
our best practices to get all the benefits of Atmos. For example, when using
our [Terraform components](https://github.com/cloudposse/terraform-aws-components), we frequently use
[Terraform Remote State](/core-concepts/components/remote-state) to retrieve the outputs from other components.
This works well when you use our components but less so when you operate in a "brownfield" environment, for example,
with an existing VPC, S3 bucket, or IAM role.

But what happens if those things weren't provisioned by Atmos or predates your infrastructure? For this reason, we
support something we refer to as the `static` remote state backend. Using the static remote state backend, you can
populate a virtual state backend with the outputs as though it had been provisioned with Terraform. You can use this
technique anytime you want to use the remote state functionality in Atmos, but when the remote state was provisioned
elsewhere.

In Atmos, brownfield development describes the process of configuring Atmos components and stacks for the
existing (already provisioned) resources, and working on and updating existing infrastructure rather than creating a new
one from scratch (which is known as "greenfield" development). The process respects the existing systems' constraints
while progressively introducing improvements and modern practices. This can ultimately lead to more robust, flexible,
and efficient systems.

Atmos supports brownfield configuration by using the remote state of type `static`.

## `static` Remote State for Brownfield Development

Suppose that we need to provision
the [`vpc`](https://github.com/cloudposse/atmos/tree/master/examples/quick-start/components/terraform/vpc)
Terraform component and, instead of provisioning an S3 bucket for VPC Flow Logs, we want to use an existing bucket.

The `vpc` Terraform component needs the outputs from the `vpc-flow-logs-bucket` Terraform component to
configure [VPC Flow Logs](https://docs.aws.amazon.com/vpc/latest/userguide/flow-logs.html).

Let's redesign the example with the `vpc` and `vpc-flow-logs-bucket` components described in
[Terraform Component Remote State](/core-concepts/components/remote-state) and configure the `static` remote state for
the `vpc-flow-logs-bucket` component to use an existing S3 bucket.

### Configure the `vpc-flow-logs-bucket` Component

In the `stacks/catalog/vpc-flow-logs-bucket.yaml` file, add the following configuration for
the `vpc-flow-logs-bucket/defaults` Atmos component:

```yaml title="stacks/catalog/vpc-flow-logs-bucket.yaml"
components:
  terraform:
    vpc-flow-logs-bucket/defaults:
      metadata:
        type: abstract
    # Use `static` remote state to configure the attributes for an existing 
    # S3 bucket for VPC Flow Logs
    remote_state_backend_type: static
    remote_state_backend:
      static:
        # ARN of the existing S3 bucket
        # `vpc_flow_logs_bucket_arn` is used as an input for the `vpc` component
        vpc_flow_logs_bucket_arn: "arn:aws:s3::/my-vpc-flow-logs-bucket"
```

<br/>

In the `stacks/ue2-dev.yaml` stack config file, add the following config for the `vpc-flow-logs-bucket-1` Atmos
component in the `ue2-dev` Atmos stack:

```yaml title="stacks/ue2-dev.yaml"
# Import the base Atmos component configuration from the `catalog`.
# `import` supports POSIX-style Globs for file names/paths (double-star `**` is supported).
# File extensions are optional (if not specified, `.yaml` is used by default).
import:
  - catalog/vpc-flow-logs-bucket

components:
  terraform:
    vpc-flow-logs-bucket-1:
      metadata:
        # Point to the Terraform component in `components/terraform` folder
        component: infra/vpc-flow-logs-bucket
        inherits:
          # Inherit all settings and variables from the 
          # `vpc-flow-logs-bucket/defaults` base Atmos component
          - vpc-flow-logs-bucket/defaults
```

<br/>

### Configure and Provision the `vpc` Component

In the `components/terraform/infra/vpc/remote-state.tf` file, configure the
[remote-state](https://github.com/cloudposse/terraform-yaml-stack-config/tree/main/modules/remote-state) Terraform
module to obtain the remote state for the `vpc-flow-logs-bucket-1` Atmos component:

```hcl title="components/terraform/infra/vpc/remote-state.tf"
module "vpc_flow_logs_bucket" {
  count = local.vpc_flow_logs_enabled ? 1 : 0

  source  = "cloudposse/stack-config/yaml//modules/remote-state"
  version = "1.5.0"

  # Specify the Atmos component name (defined in YAML stack config files) 
  # for which to get the remote state outputs
  component = var.vpc_flow_logs_bucket_component_name

  # `context` input is a way to provide the information about the stack (using the context
  # variables `namespace`, `tenant`, `environment`, `stage` defined in the stack config)
  context = module.this.context
}
```

In the `components/terraform/infra/vpc/vpc-flow-logs.tf` file, configure the `aws_flow_log` resource for the `vpc`
Terraform component to use the remote state output `vpc_flow_logs_bucket_arn` from the `vpc-flow-logs-bucket-1` Atmos
component:

```hcl title="components/terraform/infra/vpc/vpc-flow-logs.tf"
locals {
  enabled               = module.this.enabled
  vpc_flow_logs_enabled = local.enabled && var.vpc_flow_logs_enabled
}

resource "aws_flow_log" "default" {
  count = local.vpc_flow_logs_enabled ? 1 : 0

  # Use the remote state output `vpc_flow_logs_bucket_arn` of the `vpc_flow_logs_bucket` component
  log_destination = module.vpc_flow_logs_bucket[0].outputs.vpc_flow_logs_bucket_arn

  log_destination_type = var.vpc_flow_logs_log_destination_type
  traffic_type         = var.vpc_flow_logs_traffic_type
  vpc_id               = module.vpc.vpc_id

  tags = module.this.tags
}
```

In the `stacks/catalog/vpc.yaml` file, add the following default config for the `vpc/defaults` Atmos component:

```yaml title="stacks/catalog/vpc.yaml"
components:
  terraform:
    vpc/defaults:
      metadata:
        # `metadata.type: abstract` makes the component `abstract`,
        # explicitly prohibiting the component from being deployed.
        # `atmos terraform apply` will fail with an error.
        # If `metadata.type` attribute is not specified, it defaults to `real`.
        # `real` components can be provisioned by `atmos` and CI/CD like Spacelift and Atlantis.
        type: abstract
      # Default variables, which will be inherited and can be overridden in the derived components
      vars:
        public_subnets_enabled: false
        nat_gateway_enabled: false
        nat_instance_enabled: false
        max_subnet_count: 3
        vpc_flow_logs_enabled: false
        vpc_flow_logs_log_destination_type: s3
        vpc_flow_logs_traffic_type: "ALL"
```

<br/>

In the `stacks/ue2-dev.yaml` stack config file, add the following config for the `vpc/1` Atmos component in
the `ue2-dev` stack:

```yaml title="stacks/ue2-dev.yaml"
# Import the base component configuration from the `catalog`.
# `import` supports POSIX-style Globs for file names/paths (double-star `**` is supported).
# File extensions are optional (if not specified, `.yaml` is used by default).
import:
  - catalog/vpc

components:
  terraform:
    vpc/1:
      metadata:
        # Point to the Terraform component in `components/terraform` folder
        component: infra/vpc
        inherits:
          # Inherit all settings and variables from the `vpc/defaults` base Atmos component
          - vpc/defaults
      vars:
        # Define variables that are specific for this component
        # and are not set in the base component
        name: vpc-1
        ipv4_primary_cidr_block: 10.8.0.0/18
        # Override the default variables from the base component
        vpc_flow_logs_enabled: true
        vpc_flow_logs_traffic_type: "REJECT"

        # Specify the name of the Atmos component that provides configuration
        # for the `infra/vpc-flow-logs-bucket` Terraform component
        vpc_flow_logs_bucket_component_name: vpc-flow-logs-bucket-1
```

<br/>

Having the stacks configured as shown above, we can now provision the `vpc/1` Atmos component in the `ue2-dev` stack
by executing the following Atmos commands:

```shell
atmos terraform plan vpc/1 -s ue2-dev
atmos terraform apply vpc/1 -s ue2-dev
```

When the commands are executed, the `vpc_flow_logs_bucket` remote-state module detects that the `vpc-flow-logs-bucket-1`
component has the `static` remote state configured, and instead of reading its remote state from the S3 state
bucket, it just returns the static values from the `remote_state_backend.static` section.
The `vpc_flow_logs_bucket_arn` is then used as an input for the `vpc` component.
