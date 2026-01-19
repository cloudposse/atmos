# Example: Locals

Reduce repetition and build computed values using file-scoped locals.

Learn more about [Locals](https://atmos.tools/core-concepts/stacks/locals/).

## What You'll See

- **Basic locals**: Define reusable values within a file
- **Dependency resolution**: Locals can reference other locals
- **Context access**: Locals can access `settings`, `vars`, and `env` from the same file
- **Computed values**: Build complex values from simpler components

## Try It

```shell
cd examples/locals

# See how locals are resolved for dev
atmos describe component myapp -s dev

# Compare with prod (different values, same pattern)
atmos describe component myapp -s prod

# View all resolved locals
atmos describe locals myapp -s dev
```

## Example Output

```shell
$ atmos describe component myapp -s dev | grep -A5 vars:
vars:
  environment: development
  full_name: acme-development-dev
  name: acme
  tags:
    Environment: development
    ManagedBy: Atmos
    Namespace: acme
    Team: platform
```

## Key Concepts

### Locals Reference Other Locals

```yaml
locals:
  namespace: acme
  environment: dev
  # Reference other locals
  full_name: "{{ .locals.namespace }}-{{ .locals.environment }}"
```

### Locals Access Same-File Settings/Vars

```yaml
settings:
  team: platform

vars:
  stage: dev

locals:
  # Access settings and vars from THIS file only
  tag_team: "{{ .settings.team }}"
  tag_stage: "{{ .vars.stage }}"
```

### File-Scoped Isolation

Locals are resolved per-file before imports are merged. This means:
- Locals in `dev.yaml` cannot access settings from imported files
- Each file has its own independent locals scope
- Use `vars` or `settings` for cross-file sharing

## Key Files

| File | Purpose |
|------|---------|
| `stacks/deploy/dev.yaml` | Development stack with locals |
| `stacks/deploy/prod.yaml` | Production stack with different values |
| `components/terraform/myapp/main.tf` | Mock component that outputs vars |
