# Example: Demo Schemas

Validate stack configuration against JSON Schema before running Terraform.

Learn more about [Validation](https://atmos.tools/core-concepts/validate/).

## What You'll See

- [Schema from file](https://atmos.tools/core-concepts/validate/json-schema/) - local JSON Schema
- [Schema from internet](https://atmos.tools/core-concepts/validate/json-schema/#remote-schemas) - fetch from URL (schemastore.org)
- [Inline schema](https://atmos.tools/core-concepts/validate/json-schema/#inline-schemas) - embedded in atmos.yaml

## Try It

```shell
cd examples/demo-schemas

# Validate all matched files
atmos validate stacks

# See schema configuration
cat atmos.yaml
```

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Schema definitions with three source types |
| `manifest.json` | Local JSON Schema file |
| `config.yaml` | Validated against local schema |
| `bower.yaml` | Validated against remote schema |
| `inline.yaml` | Validated against inline schema |
