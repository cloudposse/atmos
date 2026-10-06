---
title: Schema Validation
tags: [Stacks]
description: >-
  Validate YAML files against JSON Schemas from three sources — a local file,
  a remote URL, and an inline schema — before anything runs.
cast:
  file: /casts/examples/demo-schemas/validate.cast
  title: atmos validate schema
related_docs:
  - label: "Validate files against schemas"
    url: /cli/commands/validate/schema
  - label: "Schema configuration"
    url: /cli/configuration/schemas
---

# Example: Demo Schemas

Validate YAML files against JSON Schema before using them in your workflows.

Learn more about [Validation](https://atmos.tools/validation/validating).

## What You'll See

- [Schema from file](https://atmos.tools/cli/configuration/schemas) - local JSON Schema
- [Schema from internet](https://atmos.tools/cli/configuration/schemas) - fetch from URL (schemastore.org)
- [Inline schema](https://atmos.tools/cli/configuration/schemas) - embedded in atmos.yaml

## Try It

```shell
cd examples/demo-schemas

# Validate all matched files against their schemas
atmos validate schema
```

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Schema definitions with three source types |
| `manifest.json` | Local JSON Schema file |
| `config.yaml` | Validated against local schema |
| `bower.yaml` | Validated against remote schema |
| `inline.yaml` | Validated against inline schema |
