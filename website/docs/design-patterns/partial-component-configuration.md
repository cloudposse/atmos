---
title: Partial Component Configuration Atmos Design Pattern
sidebar_position: 11
sidebar_label: Partial Component Configuration
description: Partial Component Configuration Atmos Design Pattern
---

# Partial Component Configuration

The **Partial Component Configuration** design pattern describes the mechanism of splitting an Atmos component configuration across many Atmos
manifests to manage, modify and apply them separately and independently in one top-level stack without affecting the others.

The mechanism is similar to [Partial Classes in
C#](https://learn.microsoft.com/en-us/dotnet/csharp/programming-guide/classes-and-structs/partial-classes-and-methods).

This is not the same as Atmos [Component Inheritance](/core-concepts/components/inheritance) where more than one Atmos component
takes part in the inheritance chain. The **Partial Component Configuration** pattern deals with one Atmos component with its configuration split
across a few configuration files.

:::note

Variations of the **Partial Component Configuration** design pattern were also implemented and described in the following patterns:

- [Component Catalog](/design-patterns/component-catalog)
- [Component Catalog with Mixins](/design-patterns/component-catalog-with-mixins)

:::

## Applicability

Use the **Partial Component Configuration** pattern when:

- You have an unbounded number of a component's instances provisioned in one environment (the same organization, OU/tenant, account and region)

- New instances of the component with different settings can be configured and provisioned anytime

- The old instances of the component must be kept unchanged and never destroyed

- You want to keep the configurations [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself)

## Structure

```console
   │   # Centralized stacks configuration (stack manifests)
   ├── stacks
   │   └── catalog
   │       └── eks
   │           └── cluster
   │               ├── defaults.yaml
   │               └── mixins
   │                   ├── k8s-1-27.yaml
   │                   ├── k8s-1-28.yaml
   │                   └── k8s-1-29.yaml
   │   # Centralized components configuration
   └── components
       └── terraform  # Terraform components (Terraform root modules)
           └── eks
               └── cluster
```

## Example

Suppose that we have EKS clusters provisioned in many accounts and regions.

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

The **Partial Component Configuration** pattern provides the following benefits:

- All settings for a component are defined in just one place (in the component's template) making the entire
  configuration [DRY](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself)

- Many instances of the component can be provisioned without repeating all the configuration values

- New Atmos components are generated dynamically

## Limitations

The **Partial Component Configuration** pattern has the following limitations and drawbacks:

- Since new Atmos components are generated dynamically, sometimes it's not easy to know the names of the Atmos components that need to be provisioned
  without looking at the `Go` template and figuring out all the Atmos component names

:::note

To address the limitations of the **Component Catalog Template** design pattern, consider the following patterns:

- [Component Catalog](/design-patterns/component-catalog)
- [Component Catalog with Mixins](/design-patterns/component-catalog-with-mixins)
- [Component Catalog Template](/design-patterns/component-catalog-template)
- [Component Inheritance](/design-patterns/component-inheritance)

:::

## Related Patterns

- [Component Catalog](/design-patterns/component-catalog)
- [Component Catalog with Mixins](/design-patterns/component-catalog-with-mixins)
- [Component Catalog Template](/design-patterns/component-catalog-template)
- [Component Inheritance](/design-patterns/component-inheritance)
- [Abstract Component](/design-patterns/abstract-component)
- [Inline Component Configuration](/design-patterns/inline-component-configuration)
- [Inline Component Customization](/design-patterns/inline-component-customization)
- [Organizational Structure Configuration](/design-patterns/organizational-structure-configuration)
