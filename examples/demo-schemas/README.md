---
title: Schema Validation
tags: [Stacks]
description: >-
  Validate YAML files against JSON Schemas from three sources — a local file,
  a remote URL, and an inline schema — before anything runs.
cast:
  file: /casts/examples/demo-schemas/validate.cast
  title: atmos validate schema
---

# Example: Demo Schemas

Validate stack configuration against JSON Schema before running Terraform.

Learn more about [Validation](https://atmos.tools/validation).

## What You'll See

- [Schema from file](https://atmos.tools/validation/json-schema) - local JSON Schema
- [Schema from internet](https://atmos.tools/validation/json-schema#remote-schemas) - fetch from URL (schemastore.org)
- [Inline schema](https://atmos.tools/validation/json-schema#inline-schemas) - embedded in atmos.yaml

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
