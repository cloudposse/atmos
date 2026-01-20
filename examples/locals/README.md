# Example: Locals

Reduce repetition and build computed values using file-scoped locals.

Learn more about [Locals](https://atmos.tools/stacks/locals).

## What You'll See

- **Basic locals**: Define reusable values within a file
- **Dependency resolution**: Locals can reference other locals
- **Context access**: Locals can access `settings`, `vars`, and `env` from the same file
- **File-scoped isolation**: Each stack file has independent locals

## Try It

```shell
cd examples/locals

# View resolved locals for the dev stack
atmos describe locals -s dev

# View resolved locals for a specific component
atmos describe locals myapp -s dev

# Compare dev vs prod
atmos describe locals myapp -s prod
```

## Key Files

| File | Purpose |
|------|---------|
| `stacks/deploy/dev.yaml` | Development stack with locals |
| `stacks/deploy/prod.yaml` | Production stack with locals |
| `components/terraform/myapp/main.tf` | Terraform component |
