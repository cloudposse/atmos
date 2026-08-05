---
title: Infracost Hook
tags: [Hooks]
cast:
  file: /casts/examples/hooks-infracost/cost-summary.cast
  title: atmos Infracost hook
---

# `hooks-infracost`

Demonstrates the **`infracost`** hook kind: an `after.terraform.plan` hook
that runs an Infracost-compatible command against the component and renders a
cost summary in the terminal.

## What this shows

- `kind: infracost` using the same hook args, output file, failure mode, and
  result handler as real Infracost.
- A local `bin/infracost-emulator` command keeps the example deterministic for
  docs and tests while still exercising the hook execution path.
- Single markdown rendering used everywhere: when Atmos Pro is connected
  the same body is uploaded; in the terminal it renders via `ui.Markdown()`.

## Requirements

- Terraform on PATH.
- **No AWS credentials needed** — the component uses dummy AWS provider
  config so `terraform plan` succeeds offline.

For real Infracost usage, remove the `command: ./bin/infracost-emulator`
override, add `dependencies.tools.infracost`, and set `INFRACOST_API_KEY`
(free at <https://www.infracost.io/>). Infracost needs that key to access its
cloud-hosted pricing database.

## Run

```bash
atmos terraform plan nat-gateway -s test
```

Expected: terraform plan succeeds; the `after.terraform.plan` hook fires; the
Infracost-compatible emulator writes the same JSON shape as `infracost
breakdown --format json`, and Atmos prints a markdown cost summary showing the
NAT gateway and EIP monthly costs.

## Files

- `atmos.yaml` — Atmos config.
- `stacks/deploy/test.yaml` — stack with a single component and one hook.
- `components/terraform/nat-gateway/` — minimal NAT gateway + EIP module.
- `bin/infracost-emulator` — deterministic executable that implements the
  output-file contract used by the Infracost hook.

## Notes

`subnet_id` is a placeholder ID. We don't need a real subnet because we
never apply — infracost prices resources from HCL/plan, not from live state.
