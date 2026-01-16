# Example: Demo Stacks

Inherit configuration across environments to eliminate duplication.

Learn more about [Stack Inheritance](https://atmos.tools/core-concepts/stacks/inheritance/).

## What You'll See

- [Catalog pattern](https://atmos.tools/core-concepts/stacks/catalogs/) with base component defaults
- [Import](https://atmos.tools/core-concepts/stacks/imports/) to inherit shared configuration
- Environment-specific [overrides](https://atmos.tools/core-concepts/stacks/overrides/) in deploy stacks

## Try It

```shell
cd examples/demo-stacks

# See how configuration is inherited
atmos describe component myapp -s dev
atmos describe component myapp -s prod

# Compare the resolved configuration across environments
atmos describe stacks --components myapp
```

## Key Files

| File | Purpose |
|------|---------|
| `stacks/catalog/myapp.yaml` | Base component configuration (shared defaults) |
| `stacks/deploy/dev.yaml` | Dev environment with imports and overrides |
| `stacks/deploy/prod.yaml` | Prod environment with different overrides |
