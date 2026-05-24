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
