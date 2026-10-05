# Example: Scaffold Directory-Level Matrix

Duplicate an entire directory's worth of files once per selection, instead of hand-maintaining a
near-duplicate directory per environment.

Learn more in the [Scaffold Command Documentation](https://atmos.tools/cli/commands/scaffold/generate).

## What You'll See

- A `spec.files[].path` glob (`"components/**"`) matching every file discovered under
  `components/`, recursively, as a single entry
- `spec.files[].matrix` expanding that one entry into one output per matched file per selected
  environment
- `.file.RelPath` in `target:` preserving each matched file's own relative position, so
  `components/vpc/main.tf` and `components/eks/main.tf` never collide once duplicated

## Try It

```shell
# List available scaffold templates
atmos scaffold list

# Duplicate components/ once per selected environment
atmos scaffold generate example ./my-project --set environments=dev,staging
```

Selecting `dev` and `staging` generates four files: `environments/dev/vpc/main.tf`,
`environments/dev/eks/main.tf`, `environments/staging/vpc/main.tf`, and
`environments/staging/eks/main.tf` — two full copies of `components/`, one per selected
environment, instead of hand-maintaining a near-duplicate directory per environment yourself.

## Key Files

| File | Purpose |
|------|---------|
| `scaffold.yaml` | Template configuration: one `environments` multiselect field, one glob+matrix-expanded directory entry |
| `components/vpc/main.tf`, `components/eks/main.tf` | Discovered template files, both duplicated by the same `"components/**"` entry into one copy per selected environment |
| `atmos.yaml` | Template for generated Atmos configuration (this whole directory is `source: "."` for the `example` template, so it's copied verbatim, like in `examples/scaffolding`) |
| `README.md` | This file, also copied verbatim into the generated project for the same reason |

## Learn More

A glob `path:` can also skip (or gate) an entire directory recursively with just `when:`, no
`matrix:` needed at all — see the
[`atmos scaffold generate`](https://atmos.tools/cli/commands/scaffold/generate) docs' "Glob Paths
and Directory-Level Matrix" section for the full reference, including the precedence rule for
when more than one entry's `path:` matches the same file.
