# `hooks-custom-command`

Demonstrates the generic **`kind: command`** hook by wiring a custom
Python script as an `after-terraform-plan` hook — the pattern for
**anything** Atmos doesn't ship a named kind for.

## What this shows

- `kind: command` runs any binary (or interpreter + script) with the
  same `ATMOS_COMPONENT_PATH` / `ATMOS_STACK` / `ATMOS_COMPONENT` /
  `ATMOS_OUTPUT_FILE` / `ATMOS_OUTPUT_DIR` env-var contract that named
  kinds use. No Go code, no contribution to Atmos required.
- `format: markdown` tells Atmos the script's `$ATMOS_OUTPUT_FILE` is
  markdown; the engine renders it via `ui.MarkdownMessage()` in the
  terminal. The same bytes flow to Pro / PR comments when those are
  connected — _format symmetry_.
- The script (`scripts/notify.py`) is intentionally trivial. In real
  use this is where you'd hit a Slack webhook, file a Jira ticket,
  append to a deployment log, or trigger a compliance check.

## Requirements

- `tofu` (OpenTofu) on PATH.
- `python3` on PATH. Available out of the box on macOS and most Linux
  distros; on Windows install from <https://python.org> or via winget.
- **No cloud credentials needed** — the demo terraform uses only the
  `random` provider, which has no cloud calls.

## Run

```bash
atmos terraform plan demo -s test
```

Expected: tofu plan creates two `random_*` resources; the
`after-terraform-plan` hook fires; the script writes a markdown table
to `$ATMOS_OUTPUT_FILE` showing all the env vars it received; Atmos
renders that markdown to your terminal.

## Adapting this

The script reads three things and writes one:

- **Reads** `ATMOS_COMPONENT_PATH`, `ATMOS_STACK`, `ATMOS_COMPONENT`
- **Writes** markdown to `$ATMOS_OUTPUT_FILE`

To plug in your own tool, mirror this:

1. Have your script (or binary) read whatever ATMOS_\* env vars it needs.
2. Write structured output to `$ATMOS_OUTPUT_FILE` (or skip it — output
   to stdout streams through Atmos's I/O layer to the user's terminal
   regardless).
3. Set `format: markdown` if the output is markdown so it gets nicely
   rendered. Otherwise omit `format:` and the artifact is just stored
   downloadably (or sent to Atmos Pro as an opaque blob).

## When to use a named kind instead

If your tool emits SARIF (security findings) or follows another
standard, prefer a named kind — `checkov`, `trivy`, `kics`, or
`infracost`. Those parse their tool's native output into structured
summaries with severity counts, cost diffs, etc. `kind: command` is
the escape hatch for everything else.
