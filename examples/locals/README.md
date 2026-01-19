# Example: Locals

Reduce repetition and build computed values using file-scoped locals.

Learn more about [Locals](https://atmos.tools/core-concepts/stacks/locals/).

## What You'll See

- **Basic locals**: Define reusable values within a file
- **Dependency resolution**: Locals can reference other locals
- **Context access**: Locals can access `settings`, `vars`, and `env`
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
$ atmos describe component myapp -s dev
```

Output (vars section showing resolved locals):

```yaml
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

### Locals Compose Values from Settings and Vars

```yaml
settings:
  team: platform

vars:
  stage: dev

locals:
  namespace: acme

  # Compose values from settings, vars, and other locals
  resource_prefix: "{{ .locals.namespace }}-{{ .vars.stage }}"
  owner_tag: "{{ .settings.team }}-team"

  # Build a tags map combining multiple sources
  default_tags:
    Namespace: "{{ .locals.namespace }}"
    Stage: "{{ .vars.stage }}"
    Owner: "{{ .locals.owner_tag }}"
```

### File-Scoped Isolation

Locals are scoped to the file where they are defined. This means:

- Locals defined in `dev.yaml` cannot be accessed from other files.
- Each file has its own independent locals scope.
- Use `vars` or `settings` for values that need to be shared across files.

## Key Files

| File                                 | Purpose                                |
|--------------------------------------|----------------------------------------|
| `stacks/deploy/dev.yaml`             | Development stack with locals          |
| `stacks/deploy/prod.yaml`            | Production stack with different values |
| `components/terraform/myapp/main.tf` | Mock component that outputs vars       |
