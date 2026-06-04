# Example: Locals

Reduce repetition and build computed values using file-scoped locals.

Learn more about [Locals](https://atmos.tools/stacks/locals).

## What You'll See

- **Basic locals** — define reusable values within a file
- **Dependency resolution** — locals reference other locals, resolved via topological sort
- **Context access** — locals use `{{ .settings.X }}`, `{{ .vars.X }}` from the same file
- **Sprig functions** — pipe syntax like `{{ .locals.namespace | upper }}`
- **Complex values** — maps with templates for resource tags
- **Multiple components** — the same locals shared across components in a file
- **File-scoped isolation** — dev.yaml and prod.yaml have independent locals

## Try It

```shell
cd examples/locals

# View resolved locals for the dev stack
atmos describe locals -s dev

# View resolved locals for a specific component
atmos describe locals myapp -s dev

# See how locals flow into component vars
atmos describe component myapp -s dev

# Compare dev vs prod (same patterns, different values)
atmos describe component myapp -s prod

# Worker component appends a suffix to locals
atmos describe component myapp-worker -s dev
```

## Features Demonstrated

### 1. Basic Locals and References

```yaml
locals:
  namespace: acme
  environment: development
  name_prefix: "{{ .locals.namespace }}-{{ .locals.environment }}"
```

Locals are resolved in dependency order — `name_prefix` waits for `namespace` and `environment`.

### 2. Settings and Vars Access

```yaml
settings:
  version: v1
vars:
  stage: dev
locals:
  app_version: "{{ .settings.version }}"    # → "v1"
  stage_name: "{{ .vars.stage }}"           # → "dev"
```

### 3. Sprig Functions

```yaml
locals:
  namespace_upper: '{{ .locals.namespace | upper }}'  # → "ACME"
```

### 4. Complex Values (Maps)

```yaml
locals:
  default_tags:
    Namespace: "{{ .locals.namespace }}"
    Environment: "{{ .locals.environment }}"
    Team: "{{ .settings.team }}"
    ManagedBy: Atmos
```

### 5. File-Scoped Isolation

Each stack file has its own locals. Even though both `dev.yaml` and `prod.yaml`
define `namespace: acme`, they are completely separate — changing one never
affects the other. Locals never propagate across file boundaries via imports.

## Key Files

| File                                 | Purpose                                   |
|--------------------------------------|-------------------------------------------|
| `stacks/deploy/dev.yaml`            | Dev stack: all locals features             |
| `stacks/deploy/prod.yaml`           | Prod stack: same patterns, different values |
| `components/terraform/myapp/main.tf` | Mock Terraform component                   |
