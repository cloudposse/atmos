# Example: Scaffold `!include`

Load a shared reference table once from a local file, instead of hand-duplicating the same list
of choices across fields, or re-deriving the same lookup data in every file that needs it.

Learn more in the [Scaffold Command Documentation](https://atmos.tools/cli/commands/scaffold/generate#loading-external-data-with-include).

## What You'll See

- A YQ-filtered `!include` shaping `lib/licenses.yaml` into real `{label, value}` options for a
  `select` field, so the choice list lives in one file instead of being typed into `scaffold.yaml`
  directly
- An unfiltered `!include` on a `type: computed` field landing the same file's raw structure at
  `.Config.license_lookup`, read from `NOTICE.md` via a plain lookup (`index`)
- `lib/licenses.yaml` — despite being declared in `spec.files[]` like any other file — never
  appearing in generated output, since it exists solely to be included

## Try It

```shell
# List available scaffold templates
atmos scaffold list

# Generate with a valid license
atmos scaffold generate example ./my-project --set license=MIT

# The options: list is real, not just documentation -- an invalid value is rejected
atmos scaffold generate example ./my-project --set license=NotARealLicense
```

The first command generates `NOTICE.md` and `atmos.yaml` (this whole directory is `source: "."`
for the `example` template, like in `examples/scaffolding`) — but never `lib/licenses.yaml`
itself. `NOTICE.md` reads:

```
# License Notice

This project is licensed under **MIT License**.

See: https://opensource.org/licenses/MIT
```

## Key Files

| File | Purpose |
|------|---------|
| `scaffold.yaml` | Template configuration: a `license` select field sourced from `!include`, a `license_lookup` computed field holding the same file's raw data |
| `lib/licenses.yaml` | The shared reference table both fields include — never copied into generated output |
| `NOTICE.md` | Discovered template file, reads `.Config.license_lookup` to resolve the selected license's full name and URL |
| `atmos.yaml` | Registers this directory as the `example` template, and is copied verbatim into generated output for the same reason `README.md` is in `examples/scaffolding` |
| `README.md` | This file, also copied verbatim into generated output |

## Learn More

`!include` resolves before schema validation runs, not just before generation — `atmos scaffold
validate` catches a missing local file, an unreachable remote source, or a YQ filter producing the
wrong shape, the same way `atmos scaffold generate` does. See the
[`atmos scaffold generate`](https://atmos.tools/cli/commands/scaffold/generate#loading-external-data-with-include)
docs for the full reference, including remote (`git::`, `oci://`, `https://`) sources.
