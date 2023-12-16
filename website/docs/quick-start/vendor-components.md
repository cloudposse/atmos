---
title: Vendor Components
sidebar_position: 5
sidebar_label: Vendor Components
---

In the previous steps, we've configured the repository and decided to provision the `vpc-flow-logs-bucket` and `vpc` Terraform
components into three AWS accounts (`dev`, `staging`, `prod`) in the two AWS regions (`us-east-2` and `us-west-2`).
We've also configured the Atmos CLI in the `atmos.yaml` CLI config file to search for the Terraform components in
the `components/terraform` directory.

Next step is to create the Terraform components `vpc-flow-logs-bucket` and `vpc`.

One way to create the Terraform components is to copy them into the corresponding folders in your repo:

- Copy the `vpc-flow-logs-bucket` component from the open-source component repository
  [vpc-flow-logs-bucket](https://github.com/cloudposse/terraform-aws-components/tree/main/modules/vpc-flow-logs-bucket)
  into the `components/terraform/vpc-flow-logs-bucket` folder

- Copy the `vpc` component from the open-source component repository
  [vpc](https://github.com/cloudposse/terraform-aws-components/tree/main/modules/vpc)
  into the `components/terraform/vpc` folder

<br/>

:::note
The recommended way to vendor the components is to execute the `atmos vendor pull` CLI command.
:::

<br/>

:::tip

For more information about Atmos Vendoring and the `atmos vendor pull` CLI command, refer to:

- [Atmos Vendoring](/core-concepts/vendoring)
- [atmos vendor pull](/cli/commands/vendor/pull)

:::

<br/>

To vendor the components from the open-source component repository [terraform-aws-components](https://github.com/cloudposse/terraform-aws-components),
perform the following steps:

- Create a `vendor.yaml` Atmos vendor config file in the root of the repo with the following content:

```yaml title="vendor.yaml"
apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: example-vendor-config
  description: Atmos vendoring manifest
spec:
  # `imports` or `sources` (or both) must be defined in a vendoring manifest
  imports: []

  sources:
    # `source` supports the following protocols: OCI (https://opencontainers.org), Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP,
    # and all URL and archive formats as described in https://github.com/hashicorp/go-getter.
    # In 'source', Golang templates are supported  https://pkg.go.dev/text/template.
    # If 'version' is provided, '{{.Version}}' will be replaced with the 'version' value before pulling the files from 'source'.
    - component: "vpc"
      source: "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref={{.Version}}"
      version: "1.343.1"
      targets:
        - "components/terraform/vpc"
      # Only include the files that match the 'included_paths' patterns.
      # If 'included_paths' is not specified, all files will be matched except those that match the patterns from 'excluded_paths'.
      # 'included_paths' support POSIX-style Globs for file names/paths (double-star `**` is supported).
      # https://en.wikipedia.org/wiki/Glob_(programming)
      # https://github.com/bmatcuk/doublestar#patterns
      included_paths:
        - "**/*.tf"
      excluded_paths:
        - "**/providers.tf"
      # Tags can be used to vendor component that have the specific tags
      # `atmos vendor pull --tags networking`
      # Refer to https://atmos.tools/cli/commands/vendor/pull
      tags:
        - networking
    - component: "vpc-flow-logs-bucket"
      source: "github.com/cloudposse/terraform-aws-components.git//modules/vpc-flow-logs-bucket?ref={{.Version}}"
      version: "1.343.1"
      targets:
        - "components/terraform/vpc-flow-logs-bucket"
      included_paths:
        - "**/*.tf"
      excluded_paths:
        - "**/providers.tf"
      # Tags can be used to vendor component that have the specific tags
      # `atmos vendor pull --tags networking,storage`
      # Refer to https://atmos.tools/cli/commands/vendor/pull
      tags:
        - storage
```

- Execute the command `atmos vendor pull` from the root of the repo

```text
Processing vendor config file 'vendor.yaml'

Pulling sources for the component 'vpc' 
from 'github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.343.1' 
into 'components/terraform/vpc'

Pulling sources for the component 'vpc-flow-logs-bucket' 
from 'github.com/cloudposse/terraform-aws-components.git//modules/vpc-flow-logs-bucket?ref=1.343.1' 
into 'components/terraform/vpc-flow-logs-bucket/1.343.1'
```

After the command is executed, the filesystem layout should look like this:

<br/>

```console
   │   # Centralized stacks configuration
   ├── stacks
   │  
   │   # Centralized components configuration. Components are broken down by tool
   └── components
       └── terraform   # Terraform components (Terraform root modules)
           ├── vpc
           │   ├── context.tf
           │   ├── main.tf
           │   ├── outputs.tf
           │   ├── providers.tf
           │   ├── remote-state.tf
           │   ├── variables.tf
           │   ├── versions.tf
           │   └── vpc-flow-logs.tf
           └── vpc-flow-logs-bucket
               ├── context.tf
               ├── main.tf
               ├── outputs.tf
               ├── providers.tf
               ├── variables.tf
               └── versions.tf
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
