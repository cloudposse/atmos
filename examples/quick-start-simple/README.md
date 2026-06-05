# Example: Quick Start Simple

Minimal Atmos setup with a single component and three environments.

Learn more in the [Quick Start Guide](https://atmos.tools/quick-start/).

## What You'll See

- Basic [stack configuration](https://atmos.tools/stacks) with dev, staging, and prod environments
- A simple Terraform [component](https://atmos.tools/components) (`weather`)
- [Catalog pattern](https://atmos.tools/howto/catalogs) for shared component defaults

## Try It

```shell
cd examples/quick-start-simple

# List all stacks
atmos list stacks

# Describe the weather component in dev
atmos describe component weather -s dev

# Plan the weather component
atmos terraform plan weather -s dev
```

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Minimal Atmos configuration |
| `stacks/deploy/` | Environment-specific stack files (dev, staging, prod) |
| `stacks/catalog/` | Shared component defaults |
| `components/terraform/weather/` | Simple Terraform component |
