---
title: Component Overrides Atmos Design Pattern
sidebar_position: 14
sidebar_label: Component Overrides
description: Component Overrides Atmos Design Pattern
---

# Component Overrides

The **Component Overrides** Design Pattern describes the mechanism of grouping Atmos components by category or function,
adding the groups of components to layers, and importing the layers into top-level Atmos stacks.

Each layer imports or configures a set of related Atmos components. Each Atmos component belongs to just one layer.
Each layer can be managed separately, possibly by different teams.

<br/>

:::tip
Refer to [Component Overrides](/core-concepts/components/overrides) for more information on the `overrides` section
:::

## Applicability

Use the **Component Overrides** pattern when:

- You have many Atmos components, and you need to group the components by category or function

- You want to split the components into layers. Each layer should be managed and modified independent of the other layers, possibly by different
  people or teams

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
   │   ├── layers (grouping of components by category/function)
   │   │   ├── load-balancers.yaml
   │   │   ├── data.yaml
   │   │   ├── dns.yaml
   │   │   ├── logs.yaml
   │   │   ├── notifications.yaml
   │   │   ├── firewalls.yaml
   │   │   ├── networking.yaml
   │   │   └── eks.yaml
   │   ├── mixins
   │   │   ├── tenant  (tenant-specific defaults)
   │   │   │   └── plat.yaml
   │   │   ├── region  (region-specific defaults)
   │   │   │   ├── us-east-2.yaml
   │   │   │   └── us-west-2.yaml
   │   │   └── stage  (stage-specific defaults)
   │   │       ├── dev.yaml
   │   │       ├── staging.yaml
   │   │       └── prod.yaml
   │   └── orgs  (organizations)
   │       └── acme
   │           ├── _defaults.yaml
   │           └── plat ('plat' OU/tenant)
   │               ├── _defaults.yaml
   │               ├── dev
   │               │   ├── _defaults.yaml
   │               │   ├── global-region.yaml
   │               │   ├── us-east-2.yaml
   │               │   └── us-west-2.yaml
   │               ├── staging
   │               │   ├── _defaults.yaml
   │               │   ├── global-region.yaml
   │               │   ├── us-east-2.yaml
   │               │   └── us-west-2.yaml
   │               └── prod
   │                   ├── _defaults.yaml
   │                   ├── global-region.yaml
   │                   ├── us-east-2.yaml
   │                   └── us-west-2.yaml
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

:::note
The **Component Overrides** Design Pattern can be applied to the configuration structures described by
the [Partial Stack Configuration](/design-patterns/partial-stack-configuration)
and [Layered Stack Configuration](/design-patterns/layered-stack-configuration)
Design Patterns.

In this example, we'll use the structure described by the [Layered Stack Configuration](/design-patterns/layered-stack-configuration)
Design Pattern.
:::

As the structure above shows, we have many different Terraform components (Terraform root modules) in the `components/terraform` folder.

In the `stacks/catalog` folder, we define the defaults for each component using the [Component Catalog](/design-patterns/component-catalog) Atmos
Design Pattern.

In the `stacks/layers` folder, we define layers (groups of components), and import the related components into the layer manifests:

- `load-balancers.yaml`
- `data.yaml`
- `dns.yaml`
- `logs.yaml`
- `notifications.yaml`
- `firewalls.yaml`
- `networking.yaml`
- `eks.yaml`

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

Add the following configuration to the `stacks/layers/load-balancers.yaml` layer manifest:

```yaml title="stacks/layers/load-balancers.yaml"
import:
  # Import the related component manifests into this layer manifest
  - catalog/alb/defaults
  # Import other Load Balancer components

# Override the configurations of all the components in this layer
# All Terraform components in this layer will get the 'Layer' and 'Team' tags
terraform:
  overrides:
    vars:
      tags:
        Layer: load-balancers
        Team: Load balancer managers
```

Add the following configuration to the `stacks/layers/data.yaml` layer manifest:

```yaml title="stacks/layers/data.yaml"
import:
  # Import the related component manifests into this layer manifest
  - catalog/aurora-postgres/defaults
  - catalog/msk/defaults
  - catalog/efs/defaults
  # Import other Data components

# Override the configurations of all the components in this layer
# All Terraform components in this layer will get the 'Layer' and 'Team' tags
terraform:
  overrides:
    vars:
      tags:
        Layer: data
        Team: Data managers
```

Add the following configuration to the `stacks/layers/dns.yaml` layer manifest:

```yaml title="stacks/layers/dns.yaml"
import:
  # Import the related component manifests into this layer manifest
  - catalog/dns/defaults
  # Import other DNS components

# Override the configurations of all the components in this layer
# All Terraform components in this layer will get the 'Layer' and 'Team' tags
terraform:
  overrides:
    vars:
      tags:
        Layer: dns
        Team: DNS managers
```

Add the following configuration to the `stacks/layers/logs.yaml` layer manifest:

```yaml title="stacks/layers/logs.yaml"
import:
  # Import the related component manifests into this layer manifest
  - catalog/network-firewall-logs-bucket/defaults
  - catalog/vpc-flow-logs-bucket/defaults
  # Import other Logs components

# Override the configurations of all the components in this layer
# All Terraform components in this layer will get the 'Layer' and 'Team' tags
terraform:
  overrides:
    vars:
      tags:
        Layer: logs
        Team: Log managers
```

Add the following configuration to the `stacks/layers/notifications.yaml` layer manifest:

```yaml title="stacks/layers/notifications.yaml"
import:
  # Import the related component manifests into this layer manifest
  - catalog/ses/defaults
  - catalog/sns-topic/defaults
  # Import other Notification components

# Override the configurations of all the components in this layer
# All Terraform components in this layer will get the 'Layer' and 'Team' tags
terraform:
  overrides:
    vars:
      tags:
        Layer: notifications
        Team: Notification managers
```

Add the following configuration to the `stacks/layers/firewalls.yaml` layer manifest:

```yaml title="stacks/layers/firewalls.yaml"
import:
  # Import the related component manifests into this layer manifest
  - catalog/network-firewall/defaults
  - catalog/waf/defaults
  # Import other Firewall components

# Override the configurations of all the components in this layer
# All Terraform components in this layer will get the 'Layer' and 'Team' tags
terraform:
  overrides:
    vars:
      tags:
        Layer: firewalls
        Team: Firewall managers
```

Add the following configuration to the `stacks/layers/networking.yaml` layer manifest:

```yaml title="stacks/layers/networking.yaml"
import:
  # Import the related component manifests into this layer manifest
  - catalog/vpc/defaults
  # Import other Networking components

# Override the configurations of all the components in this layer
# All Terraform components in this layer will get the 'Layer' and 'Team' tags
terraform:
  overrides:
    vars:
      tags:
        Layer: networking
        Team: Networking managers
```

Add the following configuration to the `stacks/layers/eks.yaml` layer manifest:

```yaml title="stacks/layers/eks.yaml"
import:
  # Import the related component manifests into this layer manifest
  - catalog/eks/defaults

# Override the configurations of all the components in this layer
# All Terraform components in this layer will get the 'Layer' and 'Team' tags
terraform:
  overrides:
    vars:
      tags:
        Layer: eks
        Team: EKS cluster managers
```

Import the required layers into the `stacks/orgs/acme/plat/dev/us-east-2.yaml` top-level stack manifest:

```yaml title="stacks/orgs/acme/plat/dev/us-east-2.yaml"
import:
  # The `orgs/acme/plat/dev/_defaults` and `mixins/region/us-east-2` manifests 
  # define the top-level Atmos stack `plat-ue2-dev`
  - orgs/acme/plat/dev/_defaults
  - mixins/region/us-east-2
  # Import the layers (groups of components)
  - layers/load-balancers
  - layers/data
  - layers/dns
  - layers/logs
  - layers/notifications
  - layers/firewalls
  - layers/networking
  - layers/eks
```

Import the required layers into the `stacks/orgs/acme/plat/dev/us-west-2.yaml` top-level stack manifest:

```yaml title="stacks/orgs/acme/plat/dev/us-west-2.yaml"
import:
  # The `orgs/acme/plat/dev/_defaults` and `mixins/region/us-west-2` manifests 
  # define the top-level Atmos stack `plat-uw2-dev`
  - orgs/acme/plat/dev/_defaults
  - mixins/region/us-west-2
  # Import the layers (groups of components)
  - layers/load-balancers
  - layers/data
  - layers/dns
  - layers/logs
  - layers/notifications
  - layers/firewalls
  - layers/networking
  - layers/eks
```

Import the required layers into the `stacks/orgs/acme/plat/prod/us-east-2.yaml` top-level stack manifest:

```yaml title="stacks/orgs/acme/plat/prod/us-east-2.yaml"
import:
  # The `orgs/acme/plat/prod/_defaults` and `mixins/region/us-east-2` manifests 
  # define the top-level Atmos stack `plat-ue2-prod`
  - orgs/acme/plat/prod/_defaults
  - mixins/region/us-east-2
  # Import the layers (groups of components)
  - layers/load-balancers
  - layers/data
  - layers/dns
  - layers/logs
  - layers/notifications
  - layers/firewalls
  - layers/networking
  - layers/eks
```

<br/>

Similarly, import the required layers into the other top-level stacks for the other organizations, OUs/tenants, accounts and regions.
Make sure to import only the layers that define the component that need to be provisioned into the stacks.

After the components are provisioned in the top-level stacks, all Terraform components will get the `Layer` and `Team` tags from the corresponding
layers.

## Benefits

The **Component Overrides** pattern provides the following benefits:

- Allows to group Atmos components by category or function

- Allows splitting the components into layers. Each layer can be managed and modified independent of the other layers, possibly by different
  people or teams

- Allows importing only the required layers into the top-level stacks (only the groups of components that need to be provisioned in the stacks)

- Makes the configurations easier to understand

## Related Patterns

- [Organizational Structure Configuration](/design-patterns/organizational-structure-configuration)
- [Partial Stack Configuration](/design-patterns/partial-stack-configuration)
- [Layered Stack Configuration](/design-patterns/layered-stack-configuration)
- [Component Catalog](/design-patterns/component-catalog)
- [Component Catalog with Mixins](/design-patterns/component-catalog-with-mixins)
