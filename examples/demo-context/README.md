---
title: Configuration Context
tags: [Stacks]
description: >-
  Compute stack names from context values with a name template, and inspect
  how Atmos resolves configuration from multiple sources.
cast:
  file: /casts/examples/demo-context/name-template.cast
  title: atmos configuration context
related_docs:
  - label: "Describe configuration"
    url: /cli/commands/describe/config
  - label: "Templates"
    url: /templates/
---

# Example: Demo Context

Inspect how Atmos resolves configuration from multiple sources.

Learn more about [Describe Config](https://atmos.tools/cli/commands/describe/config/).

## What You'll See

- [Context providers](https://atmos.tools/templates/) for dynamic values
- [Name templates](https://atmos.tools/stacks/name) using context values
- Configuration merging from imports and mixins

## Try It

```shell
cd examples/demo-context

# Stack names are computed from context values via the name template
atmos describe config --format=yaml --query .stacks.name_template

# See the stack names it produces
atmos list stacks

# Inspect the resolved component configuration (--query focuses on vars)
atmos describe component demo -s acme-west-dev --query .vars
```

> [!TIP]
> Run `atmos describe config` and `atmos describe component demo -s acme-west-dev`
> without `--query` to see the full resolved configuration.

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Name template using context providers |
| `stacks/deploy/` | Stack files with context values |
| `stacks/mixins/` | Region-specific context values |
