---
title: Partial Stack Configuration Atmos Design Pattern
sidebar_position: 12
sidebar_label: Partial Stack Configuration
description: Partial Stack Configuration Atmos Design Pattern
---

# Partial Stack Configuration

The **Partial Stack Configuration** design pattern describes the mechanism of splitting an Atmos top-level stack's configuration across many Atmos
stack manifests to manage and modify them separately and independently.

Each partial top-level stack manifest imports or configures a set of Atmos components. Each component belongs to just one of the partial top-level
stack manifests. The pattern helps to group the components per category or function and to make each partial stack manifest smaller and easier to
manage.

## Applicability

Use the **Partial Stack Configuration** pattern when:

- You have top-level stacks with complex configurations. Some parts of the configurations must be managed and modified independently of the other
  parts

- You need to group the components in a top-level stack per category or function

- You want to keep the configuration easy to manage and [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself)

## Structure

```console
   │   # Centralized stacks configuration (stack manifests)
   ├── stacks
   │   ├── catalog
   │   │   ├── alb
   │   │   │   └── defaults.yaml
   │   │   ├── aurora-postgres
   │   │   │   └── defaults.yaml
   │   │   ├── dns
   │   │   │   └── defaults.yaml
   │   │   ├── eks
   │   │   │   └── defaults.yaml
   │   │   ├── efs
   │   │   │   └── defaults.yaml
   │   │   ├── msk
   │   │   │   └── defaults.yaml
   │   │   ├── ses
   │   │   │   └── defaults.yaml
   │   │   ├── sns-topic
   │   │   │   └── defaults.yaml
   │   │   ├── network-firewall
   │   │   │   └── defaults.yaml
   │   │   ├── network-firewall-logs-bucket
   │   │   │   └── defaults.yaml
   │   │   ├── waf
   │   │   │   └── defaults.yaml
   │   │   ├── vpc
   │   │   │   └── defaults.yaml
   │   │   └── vpc-flow-logs-bucket
   │   │       └── defaults.yaml
   │   ├── mixins
   │   │   ├── tenant  (tenant-specific defaults)
   │   │   │   └── plat.yaml
   │   │   ├── region  (region-specific defaults)
   │   │   │   └── us-east-2.yaml
   │   │   └── stage  (stage-specific defaults)
   │   │       └── dev.yaml
   │   └── orgs  (organizations)
   │       └── acme
   │           ├── _defaults.yaml
   │           └── plat ('plat' OU/tenant)
   │               ├── _defaults.yaml
   │               └── dev ('dev' account)
   │                  ├── _defaults.yaml
   │                  ├── # Split the top-level stack 'plat-ue2-dev' into parts per component category
   │                  ├── us-east-2-load-balancers.yaml
   │                  ├── us-east-2-data.yaml
   │                  ├── us-east-2-dns.yaml
   │                  ├── us-east-2-logs.yaml
   │                  ├── us-east-2-notifications.yaml
   │                  ├── us-east-2-firewalls.yaml
   │                  └── us-east-2-eks.yaml
   │   # Centralized components configuration
   └── components
       └── terraform  # Terraform components (Terraform root modules)
           ├── alb
           ├── aurora-postgres
           ├── dns
           ├── eks
           ├── efs
           ├── msk
           ├── ses
           ├── sns-topic
           ├── network-firewall
           ├── network-firewall-logs-bucket
           ├── waf
           ├── vpc
           └── vpc-flow-logs-bucket
```

## Example

As the structure above shows, we have various Terraform components (Terraform root modules) in the `components/terraform` folder.

In the `stacks/catalog` folder, we define the defaults for each component using the [Component Catalog](/design-patterns/component-catalog) Atmos
Design Pattern.

In the `orgs/acme/plat/dev` folder, we split the `us-east-2` manifest into the following parts per category:

- `us-east-2-load-balancers.yaml`
- `us-east-2-data.yaml`
- `us-east-2-dns.yaml`
- `us-east-2-logs.yaml`
- `us-east-2-notifications.yaml`
- `us-east-2-firewalls.yaml`
- `us-east-2-eks.yaml`

Note that these partial stack manifests are parts of the same top-level Atmos stack `plat-ue2-dev` since they all import the same context variables
`namespace`, `tenant`, `environment` and `stage`. A top-level Atmos stack is defined by the context variables, not by the file names or locations
in the filesystem (file names can be anything, they are for people to better organize the entire configuration).

Add the following minimal configuration to `atmos.yaml` [CLI config file](/cli/configuration) :

```yaml title="atmos.yaml"
components:
  terraform:
    base_path: "components/terraform"

stacks:
  base_path: "stacks"
  name_pattern: "{tenant}-{environment}-{stage}"
  included_paths:
    # Tell Atmos to search for the top-level stack manifests in the `orgs` folder and its sub-folders
    - "orgs/**/*"
  excluded_paths:
    # Tell Atmos that the `defaults` folder and all sub-folders don't contain top-level stack manifests
    - "defaults/**/*"

schemas:
  jsonschema:
    base_path: "stacks/schemas/jsonschema"
  opa:
    base_path: "stacks/schemas/opa"
  atmos:
    manifest: "stacks/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json"
```

Add the following configuration to the `stacks/catalog/eks/clusters/defaults.yaml` manifest:

```yaml title="stacks/catalog/eks/clusters/defaults.yaml"
components:
  terraform:
    eks/cluster:
      metadata:
        # Point to the Terraform component in `components/terraform/eks/cluster`
        component: eks/cluster
      vars:
        name: eks
        availability_zones: [ ] # Use the VPC subnet AZs
        managed_node_groups_enabled: true
        node_groups:
          # will create 1 node group for each item in map
          main:
            # EKS AMI version to use, e.g. "1.16.13-20200821" (no "v").
            ami_release_version: null
            # Type of Amazon Machine Image (AMI) associated with the EKS Node Group
            ami_type: AL2_x86_64
            # Whether to enable Node Group to scale its AutoScaling Group
            cluster_autoscaler_enabled: false
            # Configure storage for the root block device for instances in the Auto Scaling Group
            block_device_map:
              "/dev/xvda":
                ebs:
                  encrypted: true
                  volume_size: 200 # GB
                  volume_type: "gp3"
            # Set of instance types associated with the EKS Node Group
            instance_types:
              - c6a.large
            # Desired number of worker nodes when initially provisioned
            desired_group_size: 2
            max_group_size: 3
            min_group_size: 2
```

## Benefits

The **Partial Stack Configuration** pattern provides the following benefits:

- Allows managing components with complex configurations where some parts of the configurations must be managed and modified independently of the
  other parts

- Different parts of component' configurations can be applied to different stacks independently of the other stacks

- Allows keeping the parts of the configurations reusable across many stacks and [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself)

## Related Patterns

- [Organizational Structure Configuration](/design-patterns/organizational-structure-configuration)
- [Layered Stack Configuration](/design-patterns/layered-stack-configuration)
- [Component Overrides](/design-patterns/component-overrides)
