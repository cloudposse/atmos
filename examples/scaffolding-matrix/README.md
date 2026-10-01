# Example: Scaffold Matrix

Generate one file per selection instead of hand-maintaining a pile of near-duplicate files.

Learn more in the [Scaffold Command Documentation](https://atmos.tools/cli/commands/scaffold/generate).

## What You'll See

- A `spec.files[].matrix` entry expanding one discovered template file into one generated file
  per selected environment
- The resolved combination available as `.matrix.<axis>` in both the output path and the
  generated file's own content

## Try It

```shell
# List available scaffold templates
atmos scaffold list

# Generate one stacks/<env>.yaml file per selected environment
atmos scaffold generate example ./my-project --set environments=dev,staging
```

Selecting `dev` and `staging` generates `stacks/dev.yaml` and `stacks/staging.yaml` — the
matrix-driven files, one per selected environment, instead of hand-maintaining a near-duplicate
file per environment yourself.

## Key Files

| File | Purpose |
|------|---------|
| `scaffold.yaml` | Template configuration: one `environments` multiselect field, one `matrix`-expanded file entry |
| `environment.yaml` | Discovered template expanded by `matrix` into one `stacks/<env>.yaml` per selected environment |
| `atmos.yaml` | Template for generated Atmos configuration (this whole directory is `source: "."` for the `example` template, so it's copied verbatim, like in `examples/scaffolding`) |
| `README.md` | This file, also copied verbatim into the generated project for the same reason |

## Learn More

An axis's values aren't limited to a literal list or a `multiselect` answer — they can also come
from a free-text answer or be computed from nested/structured answer data. See the
[`atmos scaffold generate`](https://atmos.tools/cli/commands/scaffold/generate) docs for the full
`matrix` reference.
