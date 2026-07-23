---
title: Quick Start (Simple)
tags: [Quickstart]
description: >-
  Minimal Atmos setup with a single component and three environments — no
  cloud credentials or access required.
cast:
  file: /casts/examples/quick-start-simple/list-and-plan.cast
  title: atmos quick start simple
---

# Example: Quick Start Simple

Minimal Atmos setup with a single component and three environments.

Learn more in the [Quick Start Guide](https://atmos.tools/quick-start/).

## What You'll See

- Basic [stack configuration](https://atmos.tools/stacks) with dev, staging, and prod environments
- A simple Terraform [component](https://atmos.tools/components) (`station`)
- [Catalog pattern](https://atmos.tools/howto/catalogs) for shared component defaults

## Try It

```shell
cd examples/quick-start-simple

# List all stacks
atmos list stacks

# Describe the station component in dev
atmos describe component station -s dev

# Plan every component in dev (there is one in this minimal quick start).
atmos terraform plan --all -s dev
```

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Minimal Atmos configuration |
| `stacks/deploy/` | Environment-specific stack files (dev, staging, prod) |
| `stacks/catalog/` | Shared component defaults |
| `components/terraform/station/` | Simple Terraform component |
