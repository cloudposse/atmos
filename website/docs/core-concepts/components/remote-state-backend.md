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

## Brownfield Development

[Brownfield development](https://en.wikipedia.org/wiki/Brownfield_(software_development)) is a term commonly used in the
information technology industry to describe problem spaces needing the development and deployment of new software
systems in the immediate presence of existing (legacy) software applications/systems. This implies that any new software
architecture must take into account and coexist with the existing software. The term "brownfield" itself is borrowed
from urban planning, where it describes the process of developing on previously used land that may require cleanup or
modification.

In the context of DevOps, brownfield development involves integrating new tools, practices, and technologies into
established systems. This can be challenging due to several factors:

- **Legacy Systems**: These are older technologies or systems that are still in use. They might not support modern
  practices or tools easily, and modifying them can be risky and time-consuming.

- **Complex Integrations**: Existing systems often have a complex set of integrations and dependencies which need to be
  understood and managed when new elements are introduced.

- **Cultural Shifts**: DevOps emphasizes a culture of collaboration between development and operations teams. In
  brownfield projects, shifting the organizational culture can be a significant hurdle as existing processes and
  mindsets may be deeply ingrained.

- **Technical Debt**: Over time, systems accumulate technical debt, which includes outdated code, lack of proper
  documentation, and suboptimal previous decisions that were made for expediency. Addressing technical debt is crucial
  when introducing DevOps practices to ensure that the system remains maintainable and scalable.

- **Compliance and Security**: Updating old systems often requires careful consideration of security and compliance
  issues, especially if the system handles sensitive data or must meet specific regulatory standards.

Strategies for Successful Brownfield DevOps Development:

- **Incremental Changes**: Instead of large-scale overhauls, implementing small, manageable changes can reduce risk and
  help the team learn and adapt progressively.

- **Automation**: Automating as many processes as possible, such as testing, deployments, and monitoring, can help
  integrate DevOps practices without disrupting existing operations.

- **Documentation and Training**: Proper documentation of new processes and extensive training for team members can
  facilitate a smoother transition and adoption of DevOps methodologies.

- **Tool Compatibility**: Choosing tools and technologies that can integrate well with the existing stack is essential
  to avoid disruptions and compatibility issues.

- **Continuous Feedback**: Encouraging continuous feedback from all stakeholders involved can help identify pain points
  and areas for improvement early in the development process.

## Brownfield Development in Atmos

In Atmos, brownfield development describes the process of configuring Atmos components and stacks for the
existing (already provisioned) resources, and working on and updating existing infrastructure rather than creating new
ones from scratch (which is known as "greenfield development"). The process respects the existing systems' constraints
while progressively introducing improvements and modern practices. This can ultimately lead to more robust, flexible,
and efficient systems.

Atmos supports brownfield configuration by using the `static` type of remote state.

## `static` Remote State for Brownfield Development

### Example 1

### Example 2
