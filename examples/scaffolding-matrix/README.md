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

Selecting `dev` and `staging` generates exactly `stacks/dev.yaml` and `stacks/staging.yaml` —
nothing more, nothing hand-maintained.

## Key Files

| File | Purpose |
|------|---------|
| `scaffold.yaml` | Template configuration: one `environments` multiselect field, one `matrix`-expanded file entry |
| `environment.yaml` | Discovered template expanded by `matrix` into one `stacks/<env>.yaml` per selected environment |

## Learn More

An axis's values aren't limited to a literal list or a `multiselect` answer — they can also come
from a free-text answer or be computed from nested/structured answer data. See the
[`atmos scaffold generate`](https://atmos.tools/cli/commands/scaffold/generate) docs for the full
`matrix` reference.
