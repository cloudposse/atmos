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

- You need to group the components in a stack per category or function

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

Suppose that we have EKS clusters provisioned in many accounts and regions. The clusters can run different Kubernetes versions.
Each cluster will need to be upgraded to the next Kubernetes version independently without affecting the configurations for the other clusters in
the other accounts and regions.

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

Add the following configuration to the `stacks/catalog/eks/clusters/mixins/k8s-1-27.yaml` manifest:

```yaml title="stacks/catalog/eks/clusters/mixins/k8s-1-27.yaml"
components:
  terraform:
    eks/cluster:
      vars:
        cluster_kubernetes_version: "1.27"

        # https://docs.aws.amazon.com/eks/latest/userguide/eks-add-ons.html
        # https://docs.aws.amazon.com/eks/latest/userguide/managing-add-ons.html#creating-an-add-on
        addons:
          # https://docs.aws.amazon.com/eks/latest/userguide/cni-iam-role.html
          # https://docs.aws.amazon.com/eks/latest/userguide/managing-vpc-cni.html
          # https://docs.aws.amazon.com/eks/latest/userguide/cni-iam-role.html#cni-iam-role-create-role
          # https://aws.github.io/aws-eks-best-practices/networking/vpc-cni/#deploy-vpc-cni-managed-add-on
          vpc-cni:
            addon_version: "v1.12.6-eksbuild.2" # set `addon_version` to `null` to use the latest version
            # Set default resolve_conflicts to OVERWRITE because it is required on initial installation of
            # add-ons that have self-managed versions installed by default (e.g. vpc-cni, coredns), and
            # because any custom configuration that you would want to preserve should be managed by Terraform.
            resolve_conflicts_on_create: "OVERWRITE"
            resolve_conflicts_on_update: "OVERWRITE"
          # https://docs.aws.amazon.com/eks/latest/userguide/managing-kube-proxy.html
          kube-proxy:
            addon_version: "v1.27.1-eksbuild.1" # set `addon_version` to `null` to use the latest version
            resolve_conflicts_on_create: "OVERWRITE"
            resolve_conflicts_on_update: "OVERWRITE"
          # https://docs.aws.amazon.com/eks/latest/userguide/managing-coredns.html
          coredns:
            addon_version: "v1.10.1-eksbuild.1" # set `addon_version` to `null` to use the latest version
            resolve_conflicts_on_create: "OVERWRITE"
            resolve_conflicts_on_update: "OVERWRITE"
          # https://docs.aws.amazon.com/eks/latest/userguide/csi-iam-role.html
          # https://aws.amazon.com/blogs/containers/amazon-ebs-csi-driver-is-now-generally-available-in-amazon-eks-add-ons
          # https://docs.aws.amazon.com/eks/latest/userguide/managing-ebs-csi.html#csi-iam-role
          # https://github.com/kubernetes-sigs/aws-ebs-csi-driver
          aws-ebs-csi-driver:
            addon_version: "v1.23.0-eksbuild.1" # set `addon_version` to `null` to use the latest version
            resolve_conflicts_on_create: "OVERWRITE"
            resolve_conflicts_on_update: "OVERWRITE"
            # This disables the EBS driver snapshotter sidecar and reduces the amount of logging
            # https://github.com/aws/containers-roadmap/issues/1919
            configuration_values: '{"sidecars":{"snapshotter":{"forceEnable":false}}}'
```

Import the `stacks/catalog/eks/clusters/default.yaml` and `stacks/catalog/eks/clusters/mixins/k8s-1-27.yaml` manifests into a top-level stack,
for example `stacks/orgs/acme/plat/prod/us-east-2.yaml`:

```yaml title="stacks/orgs/acme/plat/prod/us-east-2.yaml"
import:
  - orgs/acme/plat/prod/_defaults
  - mixins/region/us-east-2

  # EKS cluster configuration
  - catalog/eks/clusters/defaults
  # Import the mixin for the required Kubernetes version to define the k8s version and addon versions for the EKS cluster in this stack.
  # This is an example of partial component configuration in Atmos where the config for the component is split across many Atmos stack manifests (stack config files).
  # It's similar to Partial Classes in C# (https://learn.microsoft.com/en-us/dotnet/csharp/programming-guide/classes-and-structs/partial-classes-and-methods).
  # This is not the same as 'Atmos Component Inheritance' (https://atmos.tools/core-concepts/components/inheritance)
  # where more than one Atmos component participates in the inheritance chain.
  - catalog/eks/clusters/mixins/k8s-1-27
```

Provision the component in the stack by executing the following command:

```shell
atmos terraform apply eks/cluster -s plat-ue2-prod
```

When the `eks/cluster` component is provisioned, Atmos imports the partial component configurations from the `catalog/eks/clusters/defaults.yaml` and
`catalog/eks/clusters/mixins/k8s-1-27.yaml` manifests, deep-merges the partial configs in the order they are defined in the imports, and generates the
final variables and settings for the component in the stack.

When you need to upgrade an EKS cluster in one account and region to the next Kubernetes version, just update the imported manifest in one top-level
stack and provision the EKS cluster without affecting the clusters in the other stacks. For example, to upgrade the cluster to the next Kubernetes
version `1.28`, update the imported mixin from `catalog/eks/clusters/mixins/k8s-1-27` to `catalog/eks/clusters/mixins/k8s-1-28`. All other EKS
clusters in the other accounts and regions will stay at the current Kubernetes version `1.27` until they are ready to be upgraded.

## Benefits

The **Partial Stack Configuration** pattern provides the following benefits:

- Allows managing components with complex configurations where some parts of the configurations must be managed and modified independently of the
  other parts

- Different parts of component' configurations can be applied to different stacks independently of the other stacks

- Allows keeping the parts of the configurations reusable across many stacks and [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself)

## Related Patterns

- [Component Catalog](/design-patterns/component-catalog)
- [Component Catalog with Mixins](/design-patterns/component-catalog-with-mixins)
- [Component Catalog Template](/design-patterns/component-catalog-template)
- [Component Inheritance](/design-patterns/component-inheritance)
- [Abstract Component](/design-patterns/abstract-component)
- [Inline Component Configuration](/design-patterns/inline-component-configuration)
- [Inline Component Customization](/design-patterns/inline-component-customization)
- [Organizational Structure Configuration](/design-patterns/organizational-structure-configuration)
