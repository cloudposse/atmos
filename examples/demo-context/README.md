# Example: Demo Context

Inspect how Atmos resolves configuration from multiple sources.

Learn more about [Describe Config](https://atmos.tools/cli/commands/describe/config/).

## What You'll See

- [Context providers](https://atmos.tools/stacks/templates#context) for dynamic values
- [Name templates](https://atmos.tools/stacks/naming) using context values
- Configuration merging from imports and mixins

## Try It

```shell
cd examples/demo-context

# See the resolved Atmos configuration
atmos describe config

# Inspect component configuration
atmos describe component demo -s acme-west-dev

# See all stacks with naming
atmos describe stacks
```

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Name template using context providers |
| `stacks/deploy/` | Stack files with context values |
| `stacks/mixins/` | Region-specific context values |
