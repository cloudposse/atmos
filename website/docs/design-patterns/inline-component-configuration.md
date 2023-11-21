---
title: Inline Component Configuration Atmos Design Pattern
sidebar_position: 2
sidebar_label: Inline Component Configuration
description: Inline Component Configuration Atmos Design Pattern
---

# Inline Component Configuration

## Applicability

Use the Inline Component Configuration pattern when:

## Structure

```console
   │   # Centralized stacks configuration (stack manifests)
   ├── stacks
   │   └── dev.yaml
   │   └── staging.yaml
   │   └── prod.yaml
   │  
   │   # Centralized components configuration
   ├── components
   │   └── terraform  # Terraform components (Terraform root modules)
   │       ├── vpc
   │       ├── vpc-flow-logs-bucket
   │       ├── < other components >
```

## Example

## Benefits

The Inline Component Configuration pattern provides the following benefits:

## Drawbacks

The Inline Component Configuration pattern has the following drawbacks:

## Related Patterns

The Inline Component Configuration pattern is often implemented with:

- [Component Catalog Pattern](/design-patterns/component-catalog)
- [Component Catalog with Mixins Pattern](/design-patterns/component-catalog-with-mixins)
- [Component Catalog Template Pattern](/design-patterns/component-catalog-template)
- [Component Inheritance Pattern](/design-patterns/component-inheritance)
- [Partial Component Configuration Pattern](/design-patterns/partial-component-configuration)
