---
name: atmos-diagnostics
description: "Atmos diagnostics: machine-readable JSONL event streams, diagnostics.enabled/file/include_output, subprocess start/end/output events, masking, and debugging Atmos execution"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Diagnostics

Use this skill when debugging what Atmos is doing at runtime with machine-readable diagnostics.
Diagnostics are configuration-driven, not a standalone command.

## Configuration

Enable diagnostics in `atmos.yaml`:

```yaml
diagnostics:
  enabled: true
  file: .atmos/diagnostics/events.jsonl
  include_output: false
```

Or with environment variables:

```bash
export ATMOS_DIAGNOSTICS_ENABLED=true
export ATMOS_DIAGNOSTICS_FILE=/tmp/atmos-events.jsonl
export ATMOS_DIAGNOSTICS_INCLUDE_OUTPUT=false
```

Diagnostics are active only when `enabled` is true and `file` is non-empty.

## Event Stream

Atmos appends one JSON object per line to the configured file. Events are flat JSON for easy
inspection with `jq`, `grep`, and CI artifact upload.

Common event types include:

- `process.start`
- `process.end`
- `process.output` when `include_output: true`

Fields can include `type`, `time`, `id`, `command`, `args`, `cwd`, TTY flags, `started`,
`success`, `canceled`, `exit_code`, `duration_ms`, signal details, `error`, `stream`, `data`,
`bytes`, and `sequence`.

## Debugging Workflow

```bash
ATMOS_DIAGNOSTICS_ENABLED=true \
ATMOS_DIAGNOSTICS_FILE=/tmp/atmos-events.jsonl \
atmos terraform plan vpc -s plat-ue2-dev

jq . /tmp/atmos-events.jsonl
```

Set `include_output: true` only when command output is needed. Output events can be noisy, though
Atmos masks sensitive values before writing diagnostics.

## Guidance

- Use diagnostics when a command runs a subprocess unexpectedly, exits with an unclear code, or
  behaves differently in CI and locally.
- Attach the JSONL file as a CI artifact for failed runs.
- Keep diagnostics files out of version control.
- Diagnostics are best-effort and should not be used as a replacement for command exit codes.
- If a user asks for a CLI diagnostics command, verify current `atmos --help`; the shipped surface is
  config/env driven unless a newer command exists.
