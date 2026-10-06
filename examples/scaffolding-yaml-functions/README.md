---
related_docs:
  - label: "Scaffold generation and template configuration"
    url: /cli/commands/scaffold/generate
  - label: "Loading external data with !include and other YAML functions"
    url: /cli/commands/scaffold/generate#loading-external-data-with-include-and-other-yaml-functions
  - label: "!include YAML function"
    url: /functions/yaml/include
---

# Example: Scaffold YAML Functions

Load a shared reference table once from a local file via `!include`, instead of hand-duplicating
the same list of choices across fields or re-deriving the same lookup data in every file that
needs it — plus pull in small dynamic values like an environment variable or the current git
branch via `!env` and `!git.branch`, with no need to prompt the user for them.

Learn more in the [Scaffold Command Documentation](https://atmos.tools/cli/commands/scaffold/generate#loading-external-data-with-include-and-other-yaml-functions).

## What You'll See

- A YQ-filtered `!include` shaping `lib/licenses.yaml` into real `{label, value}` options for a
  `select` field, so the choice list lives in one file instead of being typed into `scaffold.yaml`
  directly
- An unfiltered `!include` on a `type: computed` field landing the same file's raw structure at
  `.Config.license_lookup`, read from `NOTICE.md` via a plain lookup (`index`)
- `!git.branch` and `!env` on two more `type: computed` fields, each with a fallback default so
  generation still works outside a git repository or without the environment variable set
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

# Set the environment variable !env reads, to see it flow through instead of the default
ATMOS_EXAMPLE_MAINTAINER="Jane Doe" atmos scaffold generate example ./my-project --set license=MIT
```

The first command generates `NOTICE.md` and `atmos.yaml` (this whole directory is `source: "."`
for the `example` template, like in `examples/scaffolding`) — but never `lib/licenses.yaml`
itself. `NOTICE.md` reads (branch and maintainer will vary; shown here with the defaults and
without running `atmos scaffold generate` inside a git repository):

```
# License Notice

This project is licensed under **MIT License**.

See: https://opensource.org/licenses/MIT

Generated from branch `unknown` by the project team.
```

## Key Files

| File | Purpose |
|------|---------|
| `scaffold.yaml` | Template configuration: a `license` select field sourced from `!include`, a `license_lookup` computed field holding the same file's raw data, and `generated_from_branch`/`maintainer` computed fields sourced from `!git.branch`/`!env` |
| `lib/licenses.yaml` | The shared reference table both license-related fields include — never copied into generated output |
| `NOTICE.md` | Discovered template file, reads `.Config.license_lookup`, `.Config.generated_from_branch`, and `.Config.maintainer` |
| `atmos.yaml` | Registers this directory as the `example` template, and is copied verbatim into generated output for the same reason `README.md` is in `examples/scaffolding` |
| `README.md` | This file, also copied verbatim into generated output |

## Learn More

These YAML functions resolve before schema validation runs, not just before generation —
`atmos scaffold validate` catches a missing local file, an unreachable remote source, a YQ filter
producing the wrong shape, or a malformed function call, the same way `atmos scaffold generate`
does. Only functions that need no stack, component, or backend context are supported in
`scaffold.yaml` — see the
[`atmos scaffold generate`](https://atmos.tools/cli/commands/scaffold/generate#loading-external-data-with-include-and-other-yaml-functions)
docs for the full reference, including remote (`git::`, `oci://`, `https://`) `!include` sources
and the complete supported-tag list.
