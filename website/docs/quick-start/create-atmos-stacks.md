---
title: Create Atmos Stacks
sidebar_position: 6
sidebar_label: Create Stacks
---

In the previous step, we've configured the Terraform components and described how they can be copied into the repository.

Next step is to create and configure [Atmos stacks](/core-concepts/stacks).

## Create Catalog for Components

Atmos uses the "catalog" pattern to configure default settings for Atmos components.
All the common default configs for each Atmos component should be in a separate file in the `stacks/catalog` directory.
The file then get imported into the parent Atmos stacks.
This makes the stack configurations DRY by reusing the component's config that is common for all environments.

In the `stacks/catalog/vpc-flow-logs-bucket.yaml` file, add the following default configuration for the `vpc-flow-logs-bucket-defaults` Atmos
component:

```yaml title="stacks/catalog/vpc-flow-logs-bucket.yaml"
components:
  terraform:
    vpc-flow-logs-bucket-defaults:
      metadata:
        # `metadata.type: abstract` makes the component `abstract`,
        # explicitly prohibiting the component from being deployed.
        # `atmos terraform apply` will fail with an error.
        # If `metadata.type` attribute is not specified, it defaults to `real`.
        # `real` components can be provisioned by `atmos` and CI/CD like Spacelift and Atlantis.
        type: abstract
      # Default variables, which will be inherited and can be overridden in the derived components
      vars:
        force_destroy: false
        lifecycle_rule_enabled: false
        traffic_type: "ALL"
```

In the `stacks/catalog/vpc.yaml` file, add the following default config for the `vpc-defaults` Atmos component:

```yaml title="stacks/catalog/vpc.yaml"
components:
  terraform:
    vpc-defaults:
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

These default Atmos components will be imported into the parent Atmos stacks. The default variables (in the `vars` sections) will be reused, and can
also be overridden in the derived Atmos components by using [Atmos Component Inheritance](/core-concepts/components/inheritance).

## Create Parent Stacks

When executing the [CLI commands](/cli/cheatsheet), Atmos does not use the stack file names and their filesystem locations to search for the stack
where the component is defined. Instead, Atmos uses the context variables (`namespace`, `tenant`, `environment`, `stage`) to search for the stack. The
stack config file names cam be anything, and they can be in any folders in any sub-folders in the `stacks` directory.

For example, when executing the `atmos terraform apply infra/vpc -s tenant1-ue2-dev`
command, the stack `tenant1-ue2-dev` is specified by the `-s` flag. By looking at `name_pattern: "{tenant}-{environment}-{stage}"`
(see [Configure CLI](/quick-start/configure-cli)) and processing the tokens, Atmos knows that the first part of the stack name is `tenant`, the second
part is `environment`, and the third part is `stage`. Then Atmos searches for the stack configuration file (in the `stacks` directory)
where `tenant: tenant1`, `environment: ue2` and `stage: dev` are defined (inline or via imports).
