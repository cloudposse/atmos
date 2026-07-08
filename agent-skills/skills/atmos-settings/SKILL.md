---
name: atmos-settings
description: "Atmos global settings: settings, logs, errors, env, docs, metadata, version requirements, terminal behavior, telemetry, experimental flags, and non-subsystem atmos.yaml options"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Settings

Use this skill for global `atmos.yaml` options that are not owned by a narrower subsystem skill.

## Owned Sections

| Section | Use for |
|---|---|
| `settings` | Global CLI behavior, terminal behavior, telemetry, experimental options |
| `logs` | Log level, log file path, debug output behavior |
| `errors` | Error output formatting and error reporting options |
| `env` | Environment variables applied to Atmos operations |
| `docs` | Documentation generation defaults |
| `metadata` | Project-level metadata such as name, tags, and version |
| `version` | Minimum or maximum Atmos version constraints |

If a setting belongs to a subsystem such as templates, validation, auth, CI, or toolchain, load that
subsystem skill instead.

## Common Pattern

```yaml
settings:
  terminal:
    pager: false
    syntax_highlighting:
      enabled: true
  telemetry:
    enabled: false

logs:
  level: Info

env:
  AWS_REGION: us-east-1

metadata:
  name: platform-infrastructure
```

## Terminal Appearance and Color

`settings.terminal` controls Atmos's terminal output appearance (color, theme, width, unicode,
syntax highlighting, pager). Fields (from `pkg/schema/schema.go` `Terminal` struct):

```yaml
settings:
  terminal:
    color: true               # Explicitly force color on (overrides TTY auto-detection)
    no_color: false           # Deprecated: use the --no-color flag or ATMOS_NO_COLOR/NO_COLOR/CLICOLOR env vars instead
    force_color: false        # ENV-only (ATMOS_FORCE_COLOR) -- not settable from this config field
    theme: dracula             # Theme name; see `atmos theme list` for available names
    max_width: 120
    unicode: true
    syntax_highlighting:
      enabled: true
    pager: false               # false/true/off/on/"less"/a specific pager command
```

Precedence: `no_color` (config) is deprecated in favor of the `--no-color` flag/env vars below.
`IsColorEnabled` logic: `no_color` forces color off; else explicit `color: true` forces it on;
otherwise Atmos falls back to TTY auto-detection.

### Flags and Environment Variables

| Flag | Env vars | Effect |
|---|---|---|
| `--no-color` | `ATMOS_NO_COLOR`, `NO_COLOR`, `CLICOLOR` | Disable color output |
| `--force-color` | `ATMOS_FORCE_COLOR`, `CLICOLOR_FORCE` | Force color output even when not a TTY (e.g. piped/CI output, screenshots) |

### `atmos theme` Commands

Discover and preview themes (a theme controls tables, markdown rendering, and help text colors):

```shell
atmos theme list                 # List all available themes
atmos theme list --recommended   # Only themes tested for optimal compatibility
atmos theme show <theme-name>    # Preview a theme's palette and sample UI elements
atmos theme browse               # Interactive/searchable theme gallery (docs site)
atmos list themes                # Alias for 'theme list'; defaults to recommended-only, use --all for all
```

Activate a theme via `ATMOS_THEME=<theme-name>` env var or `settings.terminal.theme` in
`atmos.yaml`.

## Routing

| Need | Load |
|---|---|
| Template settings | `atmos-templates` |
| Validation settings and policies | `atmos-validation`, `atmos-schemas` |
| Auth or identity settings | `atmos-auth` |
| Native CI settings | `atmos-ci` |
| Toolchain settings | `atmos-toolchain` |
| Stack/component `settings` fields | `atmos-stacks`, `atmos-components` |

## Guardrails

- Treat `env` as global process environment. Use component, workflow, command, or auth-specific env
  settings when only one execution context needs the variable.
- Do not put subsystem-specific details here just because they live under `settings`; route to the
  owning skill first.
- Prefer explicit, stable global settings over hidden shell assumptions.
